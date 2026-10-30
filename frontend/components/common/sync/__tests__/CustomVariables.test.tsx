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

// 自定义变量组件单元测试
// Unit tests for custom variables components

import React from 'react';
import {describe, it, expect, vi, beforeEach} from 'vitest';
import {act, render, screen, fireEvent} from '@testing-library/react';
import {
  CustomVariableDialog,
  isReservedBuiltinVariableKey,
} from '../CustomVariableDialog';
import {CustomVariablesSection} from '../CustomVariablesSection';
import {toast} from 'sonner';

// Radix Popper 依赖 ResizeObserver，jsdom 未提供 / Radix Popper needs ResizeObserver, which jsdom lacks
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

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

describe('CustomVariableDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders create dialog in string mode by default', () => {
    render(
      <CustomVariableDialog
        open={true}
        onOpenChange={vi.fn()}
        variable={null}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByText('newCustomVariable')).toBeDefined();
    expect(screen.getByText('typeString')).toBeDefined();
    expect(screen.getByText('typeSecret')).toBeDefined();
    expect(screen.getByPlaceholderText('variableKeyPlaceholder')).toBeDefined();
  });

  it('switches to secret mode and moves the security notice into a help hint', async () => {
    render(
      <CustomVariableDialog
        open={true}
        onOpenChange={vi.fn()}
        variable={null}
        onSave={vi.fn()}
      />,
    );

    const secretButton = screen.getByText('typeSecret');
    fireEvent.click(secretButton);

    // 安全说明不再常驻，收进「变量值」旁的问号 / The notice is no longer inline; it lives in the value label's help hint
    expect(screen.queryByText('secretProtectionNotice')).toBeNull();
    const hint = screen.getByTestId('custom-variable-value-secret-hint');
    await act(async () => {
      hint.focus();
      fireEvent.focus(hint);
    });
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      'secretProtectionNotice',
    );
    expect(
      screen.getByPlaceholderText('secretValueCreatePlaceholder'),
    ).toBeDefined();
  });

  it('validates required key and prevents submit', () => {
    const onSave = vi.fn();
    render(
      <CustomVariableDialog
        open={true}
        onOpenChange={vi.fn()}
        variable={null}
        onSave={onSave}
      />,
    );

    const saveButton = screen.getByText('save');
    fireEvent.click(saveButton);

    expect(toast.error).toHaveBeenCalledWith('variableKeyRequired');
    expect(onSave).not.toHaveBeenCalled();
  });

  it('validates reserved builtin keywords', () => {
    const onSave = vi.fn();
    render(
      <CustomVariableDialog
        open={true}
        onOpenChange={vi.fn()}
        variable={null}
        onSave={onSave}
      />,
    );

    const keyInput = screen.getByPlaceholderText('variableKeyPlaceholder');
    fireEvent.change(keyInput, {target: {value: 'system.biz.curdate'}});

    const saveButton = screen.getByText('save');
    fireEvent.click(saveButton);

    expect(toast.error).toHaveBeenCalledWith(
      'reservedBuiltinVariableKey:{"key":"{{system.biz.curdate}}"}',
    );
    expect(onSave).not.toHaveBeenCalled();
  });

  it('validates duplicate variable key against existing keys', () => {
    const onSave = vi.fn();
    render(
      <CustomVariableDialog
        open={true}
        onOpenChange={vi.fn()}
        variable={null}
        existingKeys={['EXISTING_KEY']}
        onSave={onSave}
      />,
    );

    const keyInput = screen.getByPlaceholderText('variableKeyPlaceholder');
    fireEvent.change(keyInput, {target: {value: 'existing_key'}});

    const saveButton = screen.getByText('save');
    fireEvent.click(saveButton);

    expect(toast.error).toHaveBeenCalledWith('duplicateCustomVariableKey');
    expect(onSave).not.toHaveBeenCalled();
  });

  it('preserves existing secret value when editing with empty input', () => {
    const onSave = vi.fn();
    render(
      <CustomVariableDialog
        open={true}
        onOpenChange={vi.fn()}
        variable={{
          id: 'var-1',
          key: 'MY_TOKEN',
          value: 'secret_token_123',
          type: 'secret',
        }}
        onSave={onSave}
      />,
    );

    expect(screen.getByText('editCustomVariable')).toBeDefined();
    expect(screen.getByText('secretEditNotice')).toBeDefined();

    const saveButton = screen.getByText('save');
    fireEvent.click(saveButton);

    expect(onSave).toHaveBeenCalledWith({
      key: 'MY_TOKEN',
      value: 'secret_token_123',
      type: 'secret',
      description: '',
    });
  });

  it('isReservedBuiltinVariableKey correctly identifies system variables', () => {
    expect(isReservedBuiltinVariableKey('system.biz.date')).toBe(true);
    expect(isReservedBuiltinVariableKey('system.task.instance.id')).toBe(true);
    expect(isReservedBuiltinVariableKey('yyyyMMdd')).toBe(true);
    expect(isReservedBuiltinVariableKey('yyyy-MM-dd')).toBe(true);
    expect(isReservedBuiltinVariableKey('yyyyMMdd-1')).toBe(true);
    expect(isReservedBuiltinVariableKey('add_months(yyyyMMdd, -1)')).toBe(true);
    expect(isReservedBuiltinVariableKey('MY_CUSTOM_VAR')).toBe(false);
    expect(isReservedBuiltinVariableKey('mysqlpass234')).toBe(false);
    expect(isReservedBuiltinVariableKey('mysqlpas')).toBe(false);
    expect(isReservedBuiltinVariableKey('password')).toBe(false);
    expect(isReservedBuiltinVariableKey('server_address')).toBe(false);
  });
});

describe('CustomVariablesSection', () => {
  it('renders empty state when no variables exist', () => {
    const onOpenCreate = vi.fn();
    render(
      <CustomVariablesSection
        variables={[]}
        onOpenCreate={onOpenCreate}
        onOpenEdit={vi.fn()}
        onDelete={vi.fn()}
        onCopyReference={vi.fn()}
        onCopyValue={vi.fn()}
      />,
    );

    expect(screen.getByText('noCustomVariables')).toBeDefined();
    const addButtons = screen.getAllByText('addCustomVariable');
    expect(addButtons.length).toBeGreaterThan(0);
    fireEvent.click(addButtons[0]);
    expect(onOpenCreate).toHaveBeenCalled();
  });

  it('renders list with secret masked badge and string value', () => {
    const onCopyRef = vi.fn();
    const onCopyVal = vi.fn();
    const onOpenEdit = vi.fn();
    const onDelete = vi.fn();

    const items = [
      {
        id: '1',
        key: 'TABLE_NAME',
        value: 'ods_orders',
        type: 'string' as const,
      },
      {
        id: '2',
        key: 'API_SECRET',
        value: 'sensitive-token',
        type: 'secret' as const,
      },
    ];

    render(
      <CustomVariablesSection
        variables={items}
        onOpenCreate={vi.fn()}
        onOpenEdit={onOpenEdit}
        onDelete={onDelete}
        onCopyReference={onCopyRef}
        onCopyValue={onCopyVal}
      />,
    );

    expect(screen.getByText('{{TABLE_NAME}}')).toBeDefined();
    expect(screen.getByText('ods_orders')).toBeDefined();
    expect(screen.getByText('stringTypeBadge')).toBeDefined();

    expect(screen.getByText('{{API_SECRET}}')).toBeDefined();
    expect(screen.getByText('secretMaskedBadge')).toBeDefined();
    expect(screen.getByText('••••••••')).toBeDefined();

    // Click reference copy
    fireEvent.click(screen.getByText('{{TABLE_NAME}}'));
    expect(onCopyRef).toHaveBeenCalledWith('TABLE_NAME');
  });
});
