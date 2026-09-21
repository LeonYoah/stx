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

// 顶层任务共享与协作组件单元测试
// Unit tests for StudioSharePopover component

import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { StudioSharePopover } from '../StudioSharePopover';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

const mockUsers = [
  { id: 1, username: 'admin', nickname: 'Administrator' },
  { id: 2, username: 'alice', nickname: 'Alice Developer' },
  { id: 3, username: 'bob', nickname: 'Bob Engineer' },
];

describe('StudioSharePopover', () => {
  it('renders trigger button with visibility status and collaborator count', () => {
    render(
      <StudioSharePopover
        isOwner={true}
        isCollaborator={false}
        canEdit={true}
        isPublic={true}
        createdBy={1}
        collaboratorIds={[2, 3]}
        workspaceUsers={mockUsers}
        isAdmin={true}
      />,
    );

    expect(screen.getByText('shareTask')).toBeInTheDocument();
    expect(screen.getByText('public')).toBeInTheDocument();
    expect(screen.getByText('+2')).toBeInTheDocument();
  });

  it('opens popover on click and allows toggling visibility and removing collaborators', () => {
    const onUpdateSharing = vi.fn();
    const onSavePermissions = vi.fn();

    render(
      <StudioSharePopover
        isOwner={true}
        isCollaborator={false}
        canEdit={true}
        isPublic={true}
        createdBy={1}
        collaboratorIds={[2]}
        workspaceUsers={mockUsers}
        isAdmin={true}
        onUpdateSharing={onUpdateSharing}
        onSavePermissions={onSavePermissions}
      />,
    );

    // 点击触发按钮展开 Popover
    // Click trigger to expand Popover
    fireEvent.click(screen.getByRole('button', { name: /shareTask/i }));

    // 验证 Popover 内部内容
    // Verify popover inner content
    expect(screen.getByText('taskSharingAndPerms')).toBeInTheDocument();
    expect(screen.getByText('taskVisibility')).toBeInTheDocument();
    expect(screen.getByText('alice')).toBeInTheDocument();

    // 切换私有状态
    // Toggle private state
    const privateBtn = screen.getByRole('button', { name: /private/i });
    fireEvent.click(privateBtn);
    expect(onUpdateSharing).toHaveBeenCalledWith(false, [2]);

    // 移除协作者
    // Remove collaborator
    const removeBtn = screen.getByTitle('removeCollaborator');
    fireEvent.click(removeBtn);
    expect(onUpdateSharing).toHaveBeenCalledWith(true, []);
  });
});
