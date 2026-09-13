// Copyright (c) 2026 SeaTunnelX
// 全局变量组件单元测试
// Unit tests for global variables components

import React from 'react';
import {describe, it, expect, vi, beforeEach} from 'vitest';
import {render, screen, fireEvent, waitFor} from '@testing-library/react';
import {GlobalVariableDialog} from '../GlobalVariableDialog';
import {GlobalVariablesSidebarPanel} from '../GlobalVariablesSidebarPanel';
import {SyncGlobalVariable} from '@/lib/services/sync/types';

// Mock next-intl
vi.mock('next-intl', () => ({
  useTranslations: () => (key: string, params?: Record<string, string | number>) => {
    if (params) {
      return `${key}:${JSON.stringify(params)}`;
    }
    return key;
  },
}));

// Mock sonner toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('GlobalVariableDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders create dialog in string mode by default', () => {
    render(
      <GlobalVariableDialog
        open={true}
        onOpenChange={vi.fn()}
        variable={null}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByText('newGlobalVariable')).toBeDefined();
    expect(screen.getByText('typeString')).toBeDefined();
    expect(screen.getByText('typeSecret')).toBeDefined();
    expect(screen.getByPlaceholderText('variableKeyPlaceholder')).toBeDefined();
  });

  it('switches to secret mode and displays security notice', async () => {
    render(
      <GlobalVariableDialog
        open={true}
        onOpenChange={vi.fn()}
        variable={null}
        onSave={vi.fn()}
      />,
    );

    const secretButton = screen.getByText('typeSecret');
    fireEvent.click(secretButton);

    expect(screen.getByText('secretProtectionNotice')).toBeDefined();
    expect(
      screen.getByPlaceholderText('secretValueCreatePlaceholder'),
    ).toBeDefined();
  });

  it('handles edit mode for an existing secret variable with empty input for retention', () => {
    const secretVar: SyncGlobalVariable = {
      id: 1,
      key: 'DB_PASSWORD',
      value: '******',
      value_type: 'secret',
      description: 'Production DB password',
      created_by: 1,
      created_at: '2026-09-13T12:00:00Z',
      updated_at: '2026-09-13T12:00:00Z',
    };

    render(
      <GlobalVariableDialog
        open={true}
        onOpenChange={vi.fn()}
        variable={secretVar}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByText('editGlobalVariable')).toBeDefined();
    expect(screen.getByText('secretEditNotice')).toBeDefined();
    const input = screen.getByPlaceholderText(
      'secretValueEditPlaceholder',
    ) as HTMLInputElement;
    expect(input.value).toBe('');
  });

  it('submits form with correct payload on save', async () => {
    const handleSave = vi.fn().mockResolvedValue(undefined);
    const handleOpenChange = vi.fn();

    render(
      <GlobalVariableDialog
        open={true}
        onOpenChange={handleOpenChange}
        variable={null}
        onSave={handleSave}
      />,
    );

    const keyInput = screen.getByPlaceholderText('variableKeyPlaceholder');
    fireEvent.change(keyInput, {target: {value: 'API_KEY'}});

    const valueInput = screen.getByPlaceholderText('variableValuePlaceholder');
    fireEvent.change(valueInput, {target: {value: 'xyz-token-789'}});

    const saveButton = screen.getByText('save');
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(handleSave).toHaveBeenCalledWith(null, {
        key: 'API_KEY',
        value: 'xyz-token-789',
        value_type: 'string',
        description: '',
      });
      expect(handleOpenChange).toHaveBeenCalledWith(false);
    });
  });
});

describe('GlobalVariablesSidebarPanel', () => {
  const mockVariables: SyncGlobalVariable[] = [
    {
      id: 1,
      key: 'JDBC_URL',
      value: 'jdbc:mysql://localhost:3306/db',
      value_type: 'string',
      description: 'MySQL database address',
      created_by: 1,
      created_at: '2026-09-13T12:00:00Z',
      updated_at: '2026-09-13T12:00:00Z',
    },
    {
      id: 2,
      key: 'DB_PASS',
      value: '******',
      value_type: 'secret',
      description: 'MySQL password',
      created_by: 1,
      created_at: '2026-09-13T12:00:00Z',
      updated_at: '2026-09-13T12:00:00Z',
    },
  ];

  it('renders variables and distinguishes between plain text and secret masking', () => {
    render(
      <GlobalVariablesSidebarPanel
        variables={mockVariables}
        total={2}
        page={1}
        pageSize={8}
        onPageChange={vi.fn()}
        onOpenCreate={vi.fn()}
        onOpenEdit={vi.fn()}
        onDelete={vi.fn()}
        onCopyValue={vi.fn()}
        onCopyReference={vi.fn()}
      />,
    );

    // Plain text variable shows value
    expect(screen.getByText('{{JDBC_URL}}')).toBeDefined();
    expect(screen.getByText('jdbc:mysql://localhost:3306/db')).toBeDefined();
    expect(screen.getByText('stringTypeBadge')).toBeDefined();

    // Secret variable shows masked bullets and badge
    expect(screen.getByText('{{DB_PASS}}')).toBeDefined();
    expect(screen.getByText('••••••••')).toBeDefined();
    expect(screen.getByText('secretMaskedBadge')).toBeDefined();
  });

  it('filters variables by type (secret)', () => {
    render(
      <GlobalVariablesSidebarPanel
        variables={mockVariables}
        total={2}
        page={1}
        pageSize={8}
        onPageChange={vi.fn()}
        onOpenCreate={vi.fn()}
        onOpenEdit={vi.fn()}
        onDelete={vi.fn()}
        onCopyValue={vi.fn()}
        onCopyReference={vi.fn()}
      />,
    );

    const secretFilterButton = screen.getByText('filterSecret');
    fireEvent.click(secretFilterButton);

    expect(screen.queryByText('{{JDBC_URL}}')).toBeNull();
    expect(screen.getByText('{{DB_PASS}}')).toBeDefined();
  });

  it('filters variables by search query', () => {
    render(
      <GlobalVariablesSidebarPanel
        variables={mockVariables}
        total={2}
        page={1}
        pageSize={8}
        onPageChange={vi.fn()}
        onOpenCreate={vi.fn()}
        onOpenEdit={vi.fn()}
        onDelete={vi.fn()}
        onCopyValue={vi.fn()}
        onCopyReference={vi.fn()}
      />,
    );

    const searchInput = screen.getByPlaceholderText('searchVariables');
    fireEvent.change(searchInput, {target: {value: 'JDBC'}});

    expect(screen.getByText('{{JDBC_URL}}')).toBeDefined();
    expect(screen.queryByText('{{DB_PASS}}')).toBeNull();
  });

  it('triggers onCopyReference when clicking reference badge', () => {
    const onCopyReference = vi.fn();

    render(
      <GlobalVariablesSidebarPanel
        variables={mockVariables}
        total={2}
        page={1}
        pageSize={8}
        onPageChange={vi.fn()}
        onOpenCreate={vi.fn()}
        onOpenEdit={vi.fn()}
        onDelete={vi.fn()}
        onCopyValue={vi.fn()}
        onCopyReference={onCopyReference}
      />,
    );

    const refButton = screen.getByText('{{JDBC_URL}}');
    fireEvent.click(refButton);

    expect(onCopyReference).toHaveBeenCalledWith('JDBC_URL');
  });
});
