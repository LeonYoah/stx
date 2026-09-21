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

// 资源树右键上下文菜单单元测试
// Unit tests for StudioTreeContextMenu component

import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { StudioTreeContextMenu } from '../StudioTreeContextMenu';
import type { SyncTaskTreeNode } from '../sync-studio-utils';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

const mockNode: SyncTaskTreeNode = {
  id: 10,
  parent_id: 1,
  node_type: 'file',
  name: 'test_task.conf',
  description: '',
  cluster_id: 1,
  engine_version: '2.3.8',
  mode: 'batch',
  status: 'draft',
  content_format: 'hocon',
  content: '',
  job_name: 'test',
  definition: {},
  sort_order: 0,
  current_version: 1,
  can_edit: true,
  can_run: true,
};

describe('StudioTreeContextMenu', () => {
  it('does not render when open is false', () => {
    const { container } = render(
      <StudioTreeContextMenu
        menuState={{
          open: false,
          x: 100,
          y: 100,
          kind: 'file',
          node: mockNode,
        }}
        onClose={vi.fn()}
        onCreateFile={vi.fn()}
        onCreateFolder={vi.fn()}
        onRename={vi.fn()}
        onMove={vi.fn()}
        onCopyFile={vi.fn()}
        onDelete={vi.fn()}
        onRefresh={vi.fn()}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders all actions and handles clicks for a file node', () => {
    const onRename = vi.fn();
    const onCopyFile = vi.fn();
    const onDelete = vi.fn();
    const onRefresh = vi.fn();
    const onClose = vi.fn();

    render(
      <StudioTreeContextMenu
        menuState={{
          open: true,
          x: 100,
          y: 100,
          kind: 'file',
          node: mockNode,
        }}
        onClose={onClose}
        onCreateFile={vi.fn()}
        onCreateFolder={vi.fn()}
        onRename={onRename}
        onMove={vi.fn()}
        onCopyFile={onCopyFile}
        onDelete={onDelete}
        onRefresh={onRefresh}
      />,
    );

    // 验证关键操作项渲染
    // Verify key action items rendered
    expect(screen.getByText('rename')).toBeInTheDocument();
    expect(screen.getByText('copyFile')).toBeInTheDocument();
    expect(screen.getByText('delete')).toBeInTheDocument();
    expect(screen.getByText('refresh')).toBeInTheDocument();

    // 点击重命名
    // Click rename
    fireEvent.click(screen.getByText('rename'));
    expect(onClose).toHaveBeenCalled();
    expect(onRename).toHaveBeenCalledWith(mockNode);

    // 点击复制文件
    // Click copy file
    fireEvent.click(screen.getByText('copyFile'));
    expect(onCopyFile).toHaveBeenCalledWith(mockNode);

    // 点击删除
    // Click delete
    fireEvent.click(screen.getByText('delete'));
    expect(onDelete).toHaveBeenCalledWith(mockNode);

    // 点击刷新
    // Click refresh
    fireEvent.click(screen.getByText('refresh'));
    expect(onRefresh).toHaveBeenCalled();
  });

  it('renders new folder and new file options for folder node', () => {
    const folderNode: SyncTaskTreeNode = {
      ...mockNode,
      id: 2,
      node_type: 'folder',
      name: 'subfolder',
    };
    const onCreateFile = vi.fn();
    const onCreateFolder = vi.fn();

    render(
      <StudioTreeContextMenu
        menuState={{
          open: true,
          x: 100,
          y: 100,
          kind: 'folder',
          node: folderNode,
        }}
        onClose={vi.fn()}
        onCreateFile={onCreateFile}
        onCreateFolder={onCreateFolder}
        onRename={vi.fn()}
        onMove={vi.fn()}
        onCopyFile={vi.fn()}
        onDelete={vi.fn()}
        onRefresh={vi.fn()}
      />,
    );

    expect(screen.getByText('newFolder')).toBeInTheDocument();
    expect(screen.getByText('newFile')).toBeInTheDocument();

    fireEvent.click(screen.getByText('newFile'));
    expect(onCreateFile).toHaveBeenCalledWith(folderNode);

    fireEvent.click(screen.getByText('newFolder'));
    expect(onCreateFolder).toHaveBeenCalledWith(folderNode);
  });
});
