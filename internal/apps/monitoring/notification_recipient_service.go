/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package monitoring

import (
	"context"
	"net/mail"
	"sort"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/auth"
)

// ListNotifiableUsers returns active users with configured email addresses.
// ListNotifiableUsers 返回已启用且配置了邮箱的可通知用户。
func (r *Repository) ListNotifiableUsers(ctx context.Context) ([]*auth.User, error) {
	var users []*auth.User
	if err := r.db.WithContext(ctx).
		Model(&auth.User{}).
		Where("is_active = ?", true).
		Where("TRIM(email) <> ''").
		Order("is_admin DESC, username ASC").
		Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// ListUsersByIDs returns users by IDs without filtering status or email.
// ListUsersByIDs 根据 ID 列表返回用户，不额外过滤状态或邮箱。
func (r *Repository) ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]*auth.User, error) {
	if len(userIDs) == 0 {
		return []*auth.User{}, nil
	}

	var users []*auth.User
	if err := r.db.WithContext(ctx).
		Model(&auth.User{}).
		Where("id IN ?", userIDs).
		Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// ListNotifiableUsers returns notification recipient directory data.
// ListNotifiableUsers 返回通知接收用户目录数据。
func (s *Service) ListNotifiableUsers(ctx context.Context) (*NotifiableUserListData, error) {
	users, defaultReceiverUserIDs, err := s.resolveNotificationRecipientDirectory(ctx)
	if err != nil {
		return nil, err
	}
	return &NotifiableUserListData{
		GeneratedAt:            timeNowUTC(),
		Users:                  users,
		DefaultReceiverUserIDs: defaultReceiverUserIDs,
	}, nil
}

func (s *Service) resolveNotificationRecipientDirectory(ctx context.Context) ([]*NotificationRecipientUserDTO, []uint64, error) {
	if s == nil || s.repo == nil {
		return []*NotificationRecipientUserDTO{}, []uint64{}, nil
	}

	rows, err := s.repo.ListNotifiableUsers(ctx)
	if err != nil {
		return nil, nil, err
	}

	users := make([]*NotificationRecipientUserDTO, 0, len(rows))
	defaultReceiverUserIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		if _, err := mail.ParseAddress(strings.TrimSpace(row.Email)); err != nil {
			continue
		}
		users = append(users, &NotificationRecipientUserDTO{
			ID:       row.ID,
			Username: strings.TrimSpace(row.Username),
			Nickname: strings.TrimSpace(row.Nickname),
			Email:    strings.TrimSpace(row.Email),
			IsAdmin:  row.IsAdmin,
		})
		if row.IsAdmin {
			defaultReceiverUserIDs = append(defaultReceiverUserIDs, row.ID)
		}
	}

	return users, normalizeReceiverUserIDs(defaultReceiverUserIDs), nil
}

func (s *Service) validateAlertPolicyReceiverUsers(ctx context.Context, receiverUserIDs []uint64) error {
	normalizedIDs := normalizeReceiverUserIDs(receiverUserIDs)
	if len(normalizedIDs) == 0 {
		return nil
	}

	rows, err := s.repo.ListUsersByIDs(ctx, normalizedIDs)
	if err != nil {
		return err
	}

	userMap := make(map[uint64]*auth.User, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		userMap[row.ID] = row
	}

	for _, receiverUserID := range normalizedIDs {
		row, exists := userMap[receiverUserID]
		if !exists {
			return ErrAlertPolicyReceiverUserNotFound
		}
		if !row.IsActive {
			return ErrAlertPolicyReceiverUserInactive
		}
		if _, err := mail.ParseAddress(strings.TrimSpace(row.Email)); err != nil {
			return ErrAlertPolicyReceiverUserEmailMissing
		}
	}
	return nil
}

func (s *Service) resolveAlertPolicyReceiverEmails(ctx context.Context, policy *AlertPolicy) ([]string, error) {
	if s == nil || s.repo == nil || policy == nil {
		return []string{}, nil
	}

	receiverUserIDs := unmarshalAlertPolicyReceiverUserIDs(policy.ReceiverUserIDsJSON)
	if len(receiverUserIDs) == 0 {
		return []string{}, nil
	}

	rows, err := s.repo.ListUsersByIDs(ctx, receiverUserIDs)
	if err != nil {
		return nil, err
	}
	userMap := make(map[uint64]*auth.User, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		userMap[row.ID] = row
	}

	recipients := make([]string, 0, len(receiverUserIDs))
	for _, receiverUserID := range receiverUserIDs {
		row, exists := userMap[receiverUserID]
		if !exists {
			return nil, ErrAlertPolicyReceiverUserNotFound
		}
		if !row.IsActive {
			return nil, ErrAlertPolicyReceiverUserInactive
		}
		email := strings.TrimSpace(row.Email)
		if _, err := mail.ParseAddress(email); err != nil {
			return nil, ErrAlertPolicyReceiverUserEmailMissing
		}
		recipients = append(recipients, email)
	}

	recipients = normalizeEmailRecipients(recipients)
	if len(recipients) == 0 {
		return nil, ErrAlertPolicyReceiverUserEmailMissing
	}
	return recipients, nil
}

func (s *Service) resolveNotificationChannelTestRecipients(
	ctx context.Context,
	channel *NotificationChannel,
	req *NotificationChannelTestRequest,
) ([]string, string, error) {
	if channel == nil {
		return []string{}, "", nil
	}
	if channel.Type != NotificationChannelTypeEmail || req == nil || req.ReceiverUserID == 0 {
		return []string{}, "", nil
	}

	rows, err := s.repo.ListUsersByIDs(ctx, []uint64{req.ReceiverUserID})
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 || rows[0] == nil {
		return nil, "", ErrAlertPolicyReceiverUserNotFound
	}

	row := rows[0]
	if !row.IsActive {
		return nil, "", ErrAlertPolicyReceiverUserInactive
	}
	email := strings.TrimSpace(row.Email)
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, "", ErrAlertPolicyReceiverUserEmailMissing
	}
	return []string{email}, email, nil
}

func normalizeReceiverUserIDs(userIDs []uint64) []uint64 {
	if len(userIDs) == 0 {
		return []uint64{}
	}

	seen := make(map[uint64]struct{}, len(userIDs))
	result := make([]uint64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 {
			continue
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		result = append(result, userID)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}
