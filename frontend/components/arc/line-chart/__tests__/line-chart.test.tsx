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

import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import {LineChart, type LineChartDatum, type LineChartSeries} from '../line-chart';

// JSDOM 环境兜底 IntersectionObserver 与 ResizeObserver，避免 Motion useInView 报错。
// Stub IntersectionObserver and ResizeObserver in JSDOM so Motion useInView runs safely.
if (typeof globalThis.IntersectionObserver === 'undefined') {
  globalThis.IntersectionObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof IntersectionObserver;
}
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

const makeSeries = (count: number): LineChartSeries[] =>
  Array.from({length: count}, (_, idx) => ({
    key: `s${idx + 1}`,
    label: idx === 0 ? "mapped - 'non-volatile memory'" : `Series ${idx + 1}`,
  }));

const makeData = (series: LineChartSeries[]): LineChartDatum[] => [
  {
    key: 't1',
    label: '10:00',
    axisLabel: '10:00',
    values: Object.fromEntries(series.map((s, idx) => [s.key, (idx + 1) * 10])),
  },
  {
    key: 't2',
    label: '10:05',
    axisLabel: '10:05',
    values: Object.fromEntries(series.map((s, idx) => [s.key, (idx + 1) * 15])),
  },
];

describe('LineChart multi-series legend', () => {
  it('legendPosition="bottom" renders the legend below the plot slider and exposes title for long labels', () => {
    const series = makeSeries(3);
    render(
      <LineChart
        label='Buffer Pool'
        data={makeData(series)}
        series={series}
        legendPosition='bottom'
      />,
    );

    const slider = screen.getByRole('slider');
    const legendGroup = screen.getByRole('group', {name: 'Buffer Pool series'});
    // 图例 DOM 顺序应位于绘图区滑块之后。/ Legend group must follow plot slider in DOM order.
    const pos = slider.compareDocumentPosition(legendGroup);
    expect(pos & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    const longToggle = within(legendGroup).getByRole('button', {
      name: "mapped - 'non-volatile memory'",
    });
    expect(longToggle).toHaveAttribute('title', "mapped - 'non-volatile memory'");
  });

  it('collapses overflowing legend toggles into a +N popover and forwards selection changes', () => {
    const series = makeSeries(8);
    const onHiddenSeriesChange = vi.fn();
    render(
      <LineChart
        label='Non-heap'
        data={makeData(series)}
        series={series}
        maxLegendItems={4}
        moreLabel={(count) => `+${count} 更多`}
        onHiddenSeriesChange={onHiddenSeriesChange}
      />,
    );

    const legendGroup = screen.getByRole('group', {name: 'Non-heap series'});
    // 行内只渲染前 4 条开关，不渲染第 5 条及之后。/ Only first 4 toggles appear inline.
    expect(
      within(legendGroup).getByRole('button', {name: 'Series 4'}),
    ).toBeInTheDocument();
    expect(
      within(legendGroup).queryByRole('button', {name: 'Series 5'}),
    ).not.toBeInTheDocument();

    const moreBtn = screen.getByTestId('line-chart-legend-more');
    expect(moreBtn).toHaveTextContent('+4 更多');

    fireEvent.click(moreBtn);
    // 弹层里应完整列出全部 8 条，并可切换第 7 条的隐藏状态。
    // Popover lists all 8 series and toggles series 7 when clicked.
    const item7 = screen.getByRole('button', {name: 'Series 7'});
    fireEvent.click(item7);
    expect(onHiddenSeriesChange).toHaveBeenCalledWith(['s7']);
  });

  it('shows emptySelectionLabel when all series are hidden and restores on reset', async () => {
    const series = makeSeries(3);
    render(
      <LineChart
        label='Heap'
        data={makeData(series)}
        series={series}
        clearLabel='清空'
        resetLabel='全选'
        emptySelectionLabel='请至少选择一条曲线'
      />,
    );

    fireEvent.click(screen.getByRole('button', {name: '清空'}));
    expect(screen.getByText('请至少选择一条曲线')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: '全选'}));
    expect(screen.getByRole('button', {name: 'Series 2'})).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await waitFor(() =>
      expect(screen.queryByText('请至少选择一条曲线')).not.toBeInTheDocument(),
    );
  });
});
