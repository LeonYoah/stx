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

import {clsx, type ClassValue} from 'clsx';
import {twMerge} from 'tailwind-merge';
import {formatDateTimeMinuteTz, formatDateTimeTz, formatDateTz} from './timezone';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * 格式化日期为可读的字符串
 * @param dateString - ISO日期字符串
 * @returns 格式化后的日期字符串
 */
export function formatDate(dateString: string): string {
  return formatDateTz(dateString, {fallback: dateString});
}

/**
 * 格式化日期时间为可读的字符串
 * @param dateString - ISO日期字符串
 * @returns 格式化后的日期时间字符串
 */
export function formatDateTime(dateString: string): string {
  return formatDateTimeMinuteTz(dateString, {fallback: dateString});
}

/**
 * 格式化日期时间为精确到秒的字符串
 * @param dateString - ISO日期字符串
 * @returns 格式化后的日期时间字符串（精确到秒）
 */
export function formatDateTimeWithSeconds(dateString: string): string {
  return formatDateTimeTz(dateString, {fallback: dateString});
}

/**
 * 复制文本到剪贴板
 * @param text - 要复制的文本
 * @returns Promise<void>
 */
export async function copyToClipboard(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    try {
      const textArea = document.createElement('textarea');
      textArea.value = text;
      textArea.style.position = 'fixed';
      textArea.style.opacity = '0';
      document.body.appendChild(textArea);
      textArea.focus();
      textArea.select();
      document.execCommand('copy');
      document.body.removeChild(textArea);
    } catch {
      throw new Error('复制失败');
    }
  }
}
