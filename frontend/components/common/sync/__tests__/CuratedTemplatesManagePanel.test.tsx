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

// 模板管理面板单元测试
// Unit tests for CuratedTemplatesManagePanel

import React from 'react';
import {describe, it, expect, vi, beforeEach} from 'vitest';
import {render, screen, fireEvent, waitFor} from '@testing-library/react';
import {CuratedTemplatesManagePanel} from '../CuratedTemplatesManagePanel';
import services from '@/lib/services';
import {copyToClipboard} from '@/lib/utils';
import {toast} from 'sonner';

// Mock next-intl
vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => {
    const messages: Record<string, string> = {
      curatedManage: '模板管理',
      curatedReadonlyEditHint: '系统模板只读，创建副本后可编辑',
      curatedOwnerOnlyHint: '只能修改和删除自己的模板',
      curatedCreate: '新建精选',
      curatedEdit: '编辑',
      curatedCopy: '复制',
      curatedCreateCopy: '创建副本',
      curatedCreateCopyHint: '基于此模板创建可编辑的副本',
      curatedCopyScript: '复制脚本',
      curatedScriptCopied: 'HOCON 脚本已复制到剪贴板',
      curatedCopyScriptFailed: '复制脚本失败',
      curatedPreview: '预览',
      curatedPreviewTitle: '模板脚本预览',
      curatedPreviewDialogDesc: '查看当前精选模板的 HOCON 定义脚本',
      curatedPreviewScriptHeading: 'HOCON 脚本内容',
      curatedToggleInlinePreview: '展开脚本',
      curatedHideInlinePreview: '收起脚本',
      curatedEditContent: '模板内容',
      curatedEditContentHint: '支持完整 HOCON 语法',
      curatedBuiltinReadonly: '系统只读',
      curatedOriginBuiltin: '系统',
      curatedOriginUser: '我的',
      curatedOriginOverride: '系统副本',
      curatedHasOverride: '已有副本',
      curatedSectionAll: '全部分区',
      curatedOriginAll: '全部来源',
      curatedSaveName: '模板名称',
      curatedSaveDescription: '描述',
      curatedSaveSection: '分区',
      confirm: '确定',
      cancel: '取消',
      close: '关闭',
      delete: '删除',
    };
    return messages[key] || key;
  },
}));

// Mock next-themes
vi.mock('next-themes', () => ({
  useTheme: () => ({resolvedTheme: 'dark'}),
}));

// Mock sonner
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
}));

// Mock copyToClipboard
vi.mock('@/lib/utils', async () => {
  const actual = await vi.importActual<typeof import('@/lib/utils')>('@/lib/utils');
  return {
    ...actual,
    copyToClipboard: vi.fn().mockResolvedValue(undefined),
  };
});

// Mock services
vi.mock('@/lib/services', () => ({
  default: {
    sync: {
      listCuratedTemplates: vi.fn(),
      renderCuratedTemplate: vi.fn(),
      forkCuratedTemplate: vi.fn(),
      createCuratedTemplate: vi.fn(),
      updateCuratedTemplate: vi.fn(),
      deleteCuratedTemplate: vi.fn(),
    },
  },
}));

// Mock @monaco-editor/react
vi.mock('@monaco-editor/react', () => ({
  __esModule: true,
  default: ({value, onChange, options}: any) => (
    <div data-testid='monaco-editor' data-readonly={String(!!options?.readOnly)}>
      <textarea
        data-testid='mock-monaco-textarea'
        value={value}
        readOnly={options?.readOnly}
        onChange={(e) => onChange?.(e.target.value)}
      />
    </div>
  ),
}));

const mockTemplates = [
  {
    id: 1,
    builtin_id: 'builtin-mysql-source',
    name: 'MySQL Source Template',
    description: 'System template for MySQL source CDC',
    section: 'source' as const,
    mode: 'batch',
    pattern: 'source',
    content: 'source {\n  MySQL-CDC {\n    result_table_name = "mysql_tbl"\n  }\n}',
    enabled: true,
    origin: 'builtin' as const,
    has_override: false,
  },
  {
    id: 2,
    name: 'My Custom Kafka Sink',
    description: 'Custom Kafka sink for audit events',
    section: 'sink' as const,
    mode: 'streaming',
    pattern: 'sink',
    content: 'sink {\n  Kafka {\n    topic = "audit_events"\n  }\n}',
    enabled: true,
    origin: 'user' as const,
  },
];

describe('CuratedTemplatesManagePanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(services.sync.listCuratedTemplates).mockResolvedValue({
      items: mockTemplates,
      total: mockTemplates.length,
    });
    vi.mocked(services.sync.renderCuratedTemplate).mockResolvedValue({
      content: 'source {\n  MySQL-CDC {\n    result_table_name = "mysql_tbl"\n  }\n}',
    });
  });

  it('renders template list with disambiguated action buttons and tags', async () => {
    render(<CuratedTemplatesManagePanel />);

    await waitFor(() => {
      expect(screen.getByText('MySQL Source Template')).toBeInTheDocument();
      expect(screen.getByText('My Custom Kafka Sink')).toBeInTheDocument();
    });

    // 验证消除了歧义的复制按钮：显式显示为「创建副本」，并提供「复制脚本」按钮
    // Verify disambiguated buttons: explicitly shown as "创建副本" (Create copy) alongside "复制脚本" (Copy Script)
    const createCopyButtons = screen.getAllByRole('button', {name: /创建副本/});
    expect(createCopyButtons.length).toBe(2);

    const copyScriptButtons = screen.getAllByRole('button', {name: /复制脚本/});
    expect(copyScriptButtons.length).toBe(2);

    // 内置模板的编辑/删除按钮为禁用态（原因在 Tooltip），自建模板可编辑与删除
    // Builtin template shows disabled edit/delete (reason in Tooltip); user template can edit & delete
    const editButtons = screen.getAllByRole('button', {name: /编辑/});
    expect(editButtons).toHaveLength(2);
    expect(editButtons.filter((b) => b.hasAttribute('disabled'))).toHaveLength(1);
    const deleteButtons = screen.getAllByRole('button', {name: /删除/});
    expect(deleteButtons).toHaveLength(2);
    expect(deleteButtons.filter((b) => b.hasAttribute('disabled'))).toHaveLength(1);
    // 标题下不再常驻说明文案 / No persistent description under the header
    expect(screen.queryByText(/查看全部精选/)).toBeNull();
  });

  it('copies HOCON script to clipboard when clicking "复制脚本"', async () => {
    render(<CuratedTemplatesManagePanel />);

    await waitFor(() => {
      expect(screen.getByText('MySQL Source Template')).toBeInTheDocument();
    });

    const copyScriptButtons = screen.getAllByRole('button', {name: /复制脚本/});
    fireEvent.click(copyScriptButtons[0]);

    await waitFor(() => {
      expect(copyToClipboard).toHaveBeenCalledWith(mockTemplates[0].content);
      expect(toast.success).toHaveBeenCalledWith('HOCON 脚本已复制到剪贴板');
    });
  });

  it('expands inline script preview when clicking "展开脚本"', async () => {
    render(<CuratedTemplatesManagePanel />);

    await waitFor(() => {
      expect(screen.getByText('MySQL Source Template')).toBeInTheDocument();
    });

    const toggleButtons = screen.getAllByRole('button', {name: /展开脚本/});
    fireEvent.click(toggleButtons[0]);

    await waitFor(() => {
      expect(screen.getByText('HOCON (source)')).toBeInTheDocument();
      expect(screen.getByText(/result_table_name = "mysql_tbl"/)).toBeInTheDocument();
    });
  });

  it('opens wide preview dialog with Monaco Editor when clicking "预览"', async () => {
    render(<CuratedTemplatesManagePanel clusterId="10" />);

    await waitFor(() => {
      expect(screen.getByText('MySQL Source Template')).toBeInTheDocument();
    });

    const previewButtons = screen.getAllByRole('button', {name: /预览/});
    fireEvent.click(previewButtons[0]);

    await waitFor(() => {
      expect(screen.getByText('HOCON 脚本内容')).toBeInTheDocument();
      const monacoEditors = screen.getAllByTestId('monaco-editor');
      expect(monacoEditors.length).toBeGreaterThan(0);
      expect(monacoEditors[0]).toHaveAttribute('data-readonly', 'true');
    });
  });

  it('opens wide edit dialog with Monaco Editor when clicking "新建精选"', async () => {
    render(<CuratedTemplatesManagePanel />);

    const createButton = screen.getByRole('button', {name: /新建精选/});
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByLabelText('模板名称')).toBeInTheDocument();
      expect(screen.getByLabelText('分区')).toBeInTheDocument();
      expect(screen.getByLabelText('描述')).toBeInTheDocument();
      expect(screen.getByText('模板内容')).toBeInTheDocument();
      const monacoEditors = screen.getAllByTestId('monaco-editor');
      expect(monacoEditors.length).toBeGreaterThan(0);
      expect(monacoEditors[0]).toHaveAttribute('data-readonly', 'false');
    });
  });
});
