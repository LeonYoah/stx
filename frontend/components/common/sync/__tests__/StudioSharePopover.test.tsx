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

const baseProps = {
  isOwner: true,
  isCollaborator: false,
  canEdit: true,
  createdBy: 1,
  workspaceUsers: mockUsers,
  isAdmin: true,
};

describe('StudioSharePopover', () => {
  it('renders trigger glyph by access level and collaborator count', () => {
    render(
      <StudioSharePopover
        {...baseProps}
        accessLevel='public_readonly'
        collaboratorIds={[2, 3]}
      />,
    );

    // 触发器小标 title 展示当前共享级别，协作者数照常计数。
    // The trigger glyph title reflects the current sharing level; collaborator count stays.
    const trigger = screen.getByRole('button', {name: 'shareTask'});
    expect(trigger).toBeInTheDocument();
    expect(
      trigger.querySelector('span[title="accessLevelPublicReadOnly"]'),
    ).not.toBeNull();
    expect(screen.getByText('+2')).toBeInTheDocument();
  });

  it('falls back to isPublic when accessLevel is absent', () => {
    render(<StudioSharePopover {...baseProps} isPublic={false} />);
    const trigger = screen.getByRole('button', {name: 'shareTask'});
    expect(
      trigger.querySelector('span[title="accessLevelPrivate"]'),
    ).not.toBeNull();
  });

  it('selects access level through the vertical radiogroup and keeps collaborator list', () => {
    const onUpdateSharing = vi.fn();
    render(
      <StudioSharePopover
        {...baseProps}
        accessLevel='public_readonly'
        collaboratorIds={[2]}
        onUpdateSharing={onUpdateSharing}
      />,
    );

    fireEvent.click(screen.getByRole('button', {name: 'shareTask'}));

    const radios = screen.getAllByRole('radio');
    expect(radios).toHaveLength(3);
    expect(radios[1]).toHaveAttribute('aria-checked', 'true');

    fireEvent.click(radios[0]);
    expect(onUpdateSharing).toHaveBeenCalledWith('private', [2]);

    fireEvent.click(radios[2]);
    expect(onUpdateSharing).toHaveBeenCalledWith('public_editable', [2]);
  });

  it('keeps collaborators editable and passes current access level on remove', () => {
    const onUpdateSharing = vi.fn();
    render(
      <StudioSharePopover
        {...baseProps}
        accessLevel='public_editable'
        collaboratorIds={[2]}
        onUpdateSharing={onUpdateSharing}
      />,
    );

    fireEvent.click(screen.getByRole('button', {name: 'shareTask'}));
    fireEvent.click(screen.getByTitle('removeCollaborator'));
    expect(onUpdateSharing).toHaveBeenCalledWith('public_editable', []);
    expect(screen.getByText('publicEditableCollaboratorHint')).toBeInTheDocument();
  });

  it('shows team editable badge for non-owner members with edit rights', () => {
    render(
      <StudioSharePopover
        isOwner={false}
        isCollaborator={false}
        canEdit={true}
        accessLevel='public_editable'
        createdBy={1}
        workspaceUsers={mockUsers}
      />,
    );
    fireEvent.click(screen.getByRole('button', {name: 'shareTask'}));
    expect(screen.getByText('teamCollaborator')).toBeInTheDocument();
    // 非管理员普通成员不可管理共享，但无 cursor-not-allowed 残留。
    // Regular members cannot manage sharing; no cursor-not-allowed styling remains.
    expect(document.querySelector('.cursor-not-allowed')).toBeNull();
  });

  it('shows read-only badge for viewers without edit rights', () => {
    render(
      <StudioSharePopover
        isOwner={false}
        isCollaborator={false}
        canEdit={false}
        accessLevel='public_readonly'
        createdBy={1}
        workspaceUsers={mockUsers}
      />,
    );
    fireEvent.click(screen.getByRole('button', {name: 'shareTask'}));
    expect(screen.getByText('readOnly')).toBeInTheDocument();
    // 只读用户的单选项禁用且无法触发切换。
    // Viewers get disabled radios and cannot switch levels.
    for (const radio of screen.getAllByRole('radio')) {
      expect(radio).toBeDisabled();
    }
  });
});
