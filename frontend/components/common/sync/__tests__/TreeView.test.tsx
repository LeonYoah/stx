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

// 文件树组件单元测试
// Unit tests for TreeView component

import React from 'react';
import {describe, it, expect, vi, beforeEach} from 'vitest';
import {render, screen, fireEvent} from '@testing-library/react';
import {TreeView} from '../TreeView';
import type {SyncTaskTreeNode} from '@/lib/services/sync';

// Mock next-intl
vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock('motion/react', async () => {
  const ReactMod = await import('react');

  function passthrough(tag: keyof HTMLElementTagNameMap) {
    return ReactMod.forwardRef(function MotionEl(
      props: Record<string, unknown> & {children?: React.ReactNode},
      ref: React.Ref<HTMLElement>,
    ) {
      const {
        children,
        layout: _layout,
        initial: _initial,
        animate: _animate,
        exit: _exit,
        transition: _transition,
        variants: _variants,
        custom: _custom,
        layoutId: _layoutId,
        layoutRoot: _layoutRoot,
        ...rest
      } = props;
      return ReactMod.createElement(tag, {ref, ...rest}, children);
    });
  }

  return {
    AnimatePresence: ({children}: {children: React.ReactNode}) => children,
    motion: {
      div: passthrough('div'),
      span: passthrough('span'),
      ul: passthrough('ul'),
    },
    useReducedMotion: () => true,
  };
});

const mockNodes: SyncTaskTreeNode[] = ([
  {
    id: 1,
    parent_id: null,
    node_type: 'folder',
    name: 'batch_jobs',
    current_version: 0,
    children: [
      {
        id: 2,
        parent_id: 1,
        node_type: 'file',
        name: 'mysql_to_hive.conf',
        current_version: 1,
        can_edit: true,
      },
      {
        id: 3,
        parent_id: 1,
        node_type: 'file',
        name: 'query.sql',
        current_version: 0,
        can_edit: true,
      },
      {
        id: 4,
        parent_id: 1,
        node_type: 'file',
        name: 'config.json',
        current_version: 2,
        can_edit: false,
      },
    ],
  },
  {
    id: 5,
    parent_id: null,
    node_type: 'folder',
    name: 'empty_folder',
    current_version: 0,
    children: [],
  },
] as unknown as SyncTaskTreeNode[]);

describe('TreeView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders folder and child files correctly when expanded', () => {
    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[1]}
        onSelect={vi.fn()}
        onContextMenu={vi.fn()}
      />,
    );

    expect(screen.getByRole('tree', {name: 'resources'})).toBeInTheDocument();
    expect(screen.getByText('batch_jobs')).toBeInTheDocument();
    expect(screen.getByText('empty_folder')).toBeInTheDocument();
    expect(screen.getByText('mysql_to_hive.conf')).toBeInTheDocument();
    expect(screen.getByText('query.sql')).toBeInTheDocument();
    expect(screen.getByText('config.json')).toBeInTheDocument();
  });

  it('reports expanded folder ids when a folder row is toggled', () => {
    const onSelect = vi.fn();
    const onExpandedChange = vi.fn();
    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[]}
        onSelect={onSelect}
        onExpandedChange={onExpandedChange}
        onContextMenu={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText('empty_folder'));
    expect(onSelect).toHaveBeenCalledWith(mockNodes[1]);
    expect(onExpandedChange).toHaveBeenCalledWith([5]);
  });

  it('handles node selection on click', () => {
    const onSelect = vi.fn();
    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[1]}
        onSelect={onSelect}
        onContextMenu={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText('mysql_to_hive.conf'));
    expect(onSelect).toHaveBeenCalledWith(mockNodes[0].children![0]);
  });

  it('triggers onRenameStart on double click', () => {
    const onRenameStart = vi.fn();
    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[1]}
        onSelect={vi.fn()}
        onContextMenu={vi.fn()}
        onRenameStart={onRenameStart}
      />,
    );

    fireEvent.doubleClick(screen.getByText('mysql_to_hive.conf'));
    expect(onRenameStart).toHaveBeenCalledWith(mockNodes[0].children![0]);
  });

  it('does not trigger onRenameStart on double click if node is read-only', () => {
    const onRenameStart = vi.fn();
    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[1]}
        onSelect={vi.fn()}
        onContextMenu={vi.fn()}
        onRenameStart={onRenameStart}
      />,
    );

    // config.json has can_edit: false
    fireEvent.doubleClick(screen.getByText('config.json'));
    expect(onRenameStart).not.toHaveBeenCalled();
  });

  it('renders inline rename input and commits on Enter', () => {
    const onRenameCommit = vi.fn();
    const onRenameCancel = vi.fn();
    render(
      <TreeView
        nodes={mockNodes[0].children!}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[]}
        onSelect={vi.fn()}
        onContextMenu={vi.fn()}
        renamingNodeId={2}
        onRenameCommit={onRenameCommit}
        onRenameCancel={onRenameCancel}
      />,
    );

    const input = screen.getByDisplayValue('mysql_to_hive.conf');
    expect(input).toBeInTheDocument();

    fireEvent.change(input, {target: {value: 'mysql_to_clickhouse.conf'}});
    fireEvent.keyDown(input, {key: 'Enter'});

    expect(onRenameCommit).toHaveBeenCalledWith(
      mockNodes[0].children![0],
      'mysql_to_clickhouse.conf',
    );
  });

  it('does not treat rename input keys as tree navigation', () => {
    const onSelect = vi.fn();
    const onExpandedChange = vi.fn();

    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[1]}
        onSelect={onSelect}
        onExpandedChange={onExpandedChange}
        onContextMenu={vi.fn()}
        renamingNodeId={1}
      />,
    );

    const input = screen.getByDisplayValue('batch_jobs');
    fireEvent.keyDown(input, {key: 'ArrowDown'});
    fireEvent.keyDown(input, {key: 'Enter'});

    expect(onSelect).not.toHaveBeenCalled();
    expect(onExpandedChange).not.toHaveBeenCalled();
  });

  it('cancels inline rename on Escape key', () => {
    const onRenameCancel = vi.fn();
    render(
      <TreeView
        nodes={mockNodes[0].children!}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[]}
        onSelect={vi.fn()}
        onContextMenu={vi.fn()}
        renamingNodeId={2}
        onRenameCancel={onRenameCancel}
      />,
    );

    const input = screen.getByDisplayValue('mysql_to_hive.conf');
    fireEvent.keyDown(input, {key: 'Escape'});

    expect(onRenameCancel).toHaveBeenCalled();
  });

  it('renders inline creation row and commits on Enter', () => {
    const onCreateCommit = vi.fn();
    const onCreateCancel = vi.fn();
    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[1]}
        onSelect={vi.fn()}
        onContextMenu={vi.fn()}
        creatingNode={{parentId: 1, nodeType: 'file'}}
        onCreateCommit={onCreateCommit}
        onCreateCancel={onCreateCancel}
      />,
    );

    const input = screen.getByPlaceholderText('newFile');
    expect(input).toBeInTheDocument();

    fireEvent.change(input, {target: {value: 'new_job.conf'}});
    fireEvent.keyDown(input, {key: 'Enter'});

    expect(onCreateCommit).toHaveBeenCalledWith('new_job.conf');
  });

  it('truncates file names and keeps version and draft badges on the row', () => {
    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[1]}
        onSelect={vi.fn()}
        onContextMenu={vi.fn()}
      />,
    );

    const name = screen.getByText('mysql_to_hive.conf');
    expect(name).toHaveAttribute('data-slot', 'tree-node-name');
    expect(name.className).toMatch(/truncate/);
    expect(name.className).toMatch(/min-w-0/);
    expect(name.closest('[role="treeitem"]')?.className).toMatch(/min-w-0/);

    const published = screen.getByText('v1');
    const draft = screen.getByText('draft');
    expect(published).toHaveAttribute('data-slot', 'tree-node-badge');
    expect(draft).toHaveAttribute('data-slot', 'tree-node-badge');
    expect(published.className).toMatch(/shrink-0/);
    expect(draft.className).toMatch(/whitespace-nowrap/);
    expect(screen.getByText('v2')).toBeInTheDocument();
  });

  it('does not pin create or delete icons onto tree rows', () => {
    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[]}
        onSelect={vi.fn()}
        onContextMenu={vi.fn()}
      />,
    );

    expect(screen.queryByRole('button', {name: 'newFile'})).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', {name: 'newFolder'}),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'delete'})).not.toBeInTheDocument();
    expect(screen.getByText('batch_jobs')).toBeInTheDocument();
  });

  it('supports drag and drop events', () => {
    const onDragStart = vi.fn();
    const onDragOver = vi.fn();
    const onDrop = vi.fn();

    render(
      <TreeView
        nodes={mockNodes}
        selectedNodeId={null}
        selectedFolderId={null}
        expandedFolderIds={[1]}
        onSelect={vi.fn()}
        onContextMenu={vi.fn()}
        onDragStart={onDragStart}
        onDragOver={onDragOver}
        onDrop={onDrop}
        draggingNodeId={2}
        dragOverFolderId={5}
      />,
    );

    const fileNode = screen.getByText('mysql_to_hive.conf');
    const folderNode = screen.getByText('empty_folder');

    fireEvent.dragStart(fileNode, {
      dataTransfer: {setData: vi.fn(), effectAllowed: ''},
    });
    expect(onDragStart).toHaveBeenCalledWith(mockNodes[0].children![0]);

    fireEvent.dragOver(folderNode, {
      dataTransfer: {dropEffect: ''},
    });
    expect(onDragOver).toHaveBeenCalledWith(mockNodes[1]);

    fireEvent.drop(folderNode);
    expect(onDrop).toHaveBeenCalledWith(mockNodes[1]);
  });
});
