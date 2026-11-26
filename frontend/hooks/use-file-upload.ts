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

'use client';

import {useState, useCallback} from 'react';
import {toast} from '@/lib/toast';

/**
 * 检测文件内容是否为二进制文件
 */
const isBinaryFile = (content: string): boolean => {
  // 检查是否包含 null 字符（二进制文件常见特征）
  if (content.includes('\0')) {
    return true;
  }

  // 检查非打印字符的比例
  const totalLength = content.length;
  if (totalLength === 0) {return false;}

  let nonPrintableCount = 0;
  for (let i = 0; i < Math.min(totalLength, 1000); i++) {
    const charCode = content.charCodeAt(i);
    // 检查是否为非打印字符（除了换行符、回车符、制表符）
    if (charCode < 32 && charCode !== 9 && charCode !== 10 && charCode !== 13) {
      nonPrintableCount++;
    }
  }

  // 如果非打印字符超过 5%，认为是二进制文件
  return nonPrintableCount / Math.min(totalLength, 1000) > 0.05;
};

/**
 * 检测文件内容是否包含非 ASCII 字符
 */
const hasNonAsciiCharacters = (content: string): boolean => {
  for (let i = 0; i < content.length; i++) {
    const charCode = content.charCodeAt(i);
    if (charCode > 127) {
      return true;
    }
  }
  return false;
};

/**
 * 检测文件内容是否包含非 Unicode 字符
 */
const hasNonUnicodeCharacters = (content: string): boolean => {
  try {
    // 尝试将字符串转换为 UTF-8 并检查是否有替换字符
    const encoded = new TextEncoder().encode(content);
    const decoded = new TextDecoder('utf-8', {fatal: true}).decode(encoded);
    return decoded !== content;
  } catch {
    // 如果解码失败，说明包含非 Unicode 字符
    return true;
  }
};

/**
 * 检测文件内容特征
 */
const analyzeFileContent = (
  content: string,
): {
  isBinary: boolean;
  hasNonAscii: boolean;
  hasNonUnicode: boolean;
} => {
  const isBinary = isBinaryFile(content);
  const hasNonAscii = hasNonAsciiCharacters(content);
  const hasNonUnicode = hasNonUnicodeCharacters(content);

  return {
    isBinary,
    hasNonAscii,
    hasNonUnicode,
  };
};

/**
 * 解析 JSONL 文件内容
 * 支持两种格式：
 * 1. JSON 数组格式：[{}, {}, {}]
 * 2. 每行一个 JSON 对象
 */
const parseJsonlContent = (content: string): string => {
  const trimmedContent = content.trim();

  // 尝试解析为 JSON 数组格式
  if (trimmedContent.startsWith('[') && trimmedContent.endsWith(']')) {
    try {
      const jsonArray = JSON.parse(trimmedContent);
      if (Array.isArray(jsonArray)) {
        return jsonArray
          .map((item) => {
            if (typeof item === 'object' && item !== null) {
              return JSON.stringify(item);
            }
            return String(item);
          })
          .filter((item) => item.trim())
          .join('\n');
      }
    } catch {
      // JSON 数组解析失败，继续尝试逐行解析
    }
  }

  // 逐行解析 JSON 对象
  const lines = trimmedContent
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);

  const validJsonLines: string[] = [];

  for (const line of lines) {
    try {
      // 尝试解析为 JSON 对象
      const jsonObj = JSON.parse(line);
      if (typeof jsonObj === 'object' && jsonObj !== null) {
        validJsonLines.push(JSON.stringify(jsonObj));
      } else {
        // 非对象类型，直接转为字符串
        validJsonLines.push(String(jsonObj));
      }
    } catch {
      // JSON 解析失败，作为普通文本处理
      validJsonLines.push(line);
    }
  }

  return validJsonLines.join('\n');
};

export function useFileUpload() {
  const [fileUploadOpen, setFileUploadOpen] = useState(false);
  const [confirmationOpen, setConfirmationOpen] = useState(false);
  const [pendingFile, setPendingFile] = useState<{
    file: File;
    content: string;
    analysis: {
      isBinary: boolean;
      hasNonAscii: boolean;
      hasNonUnicode: boolean;
    };
    currentItems: string[];
    allowDuplicates: boolean;
    onSuccess: (
      newItems: string[],
      importedCount: number,
      skippedInfo?: string,
    ) => void;
  } | null>(null);

  const processFileContent = useCallback(
    (
      content: string,
      file: File,
      currentItems: string[],
      allowDuplicates: boolean,
      onSuccess: (
        newItems: string[],
        importedCount: number,
        skippedInfo?: string,
      ) => void,
    ) => {
      const fileName = file.name.toLowerCase();
      let processedContent = content;

      // 根据文件扩展名选择解析方式
      if (fileName.endsWith('.jsonl')) {
        // JSONL 文件处理
        processedContent = parseJsonlContent(content);
      } else {
        // TXT 文件处理（保持原有逻辑）
        const lines = content
          .split(/\r?\n/)
          .map((line) => line.trim())
          .filter((line) => line.length > 0);

        if (lines.length === 0) {
          toast.error('文件内容为空');
          return;
        }

        processedContent = lines.join('\n');
      }

      if (!processedContent) {
        toast.error('文件内容为空或格式无效');
        return;
      }

      // 执行导入（本地实现，避免依赖不存在的组件）
      const lines = processedContent
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => line.length > 0);

      if (lines.length === 0) {
        toast.error('文件内容为空或格式无效');
        return;
      }

      let importedCount = 0;
      let skippedDuplicates = 0;
      const newItems = [...currentItems];

      for (const line of lines) {
        if (!allowDuplicates && newItems.includes(line)) {
          skippedDuplicates++;
          continue;
        }
        newItems.push(line);
        importedCount++;
      }

      if (importedCount === 0) {
        toast.error('没有新内容被导入');
        return;
      }

      const skippedInfo =
        skippedDuplicates > 0 ? `（跳过重复 ${skippedDuplicates} 条）` : '';

      onSuccess(newItems, importedCount, skippedInfo);
      const message = `从文件成功导入 ${importedCount} 个内容${skippedInfo}`;
      toast.success(message);
      setFileUploadOpen(false);
    },
    [],
  );

  const handleConfirmUpload = useCallback(() => {
    if (pendingFile) {
      processFileContent(
        pendingFile.content,
        pendingFile.file,
        pendingFile.currentItems,
        pendingFile.allowDuplicates,
        pendingFile.onSuccess,
      );
      setPendingFile(null);
      setConfirmationOpen(false);
    }
  }, [pendingFile, processFileContent]);

  const handleCancelUpload = useCallback(() => {
    setPendingFile(null);
    setConfirmationOpen(false);
  }, []);

  const handleFileUpload = useCallback(
    (
      files: File[],
      currentItems: string[],
      allowDuplicates: boolean,
      onSuccess: (
        newItems: string[],
        importedCount: number,
        skippedInfo?: string,
      ) => void,
    ) => {
      if (files.length === 0) {return;}

      const file = files[0];
      const fileName = file.name.toLowerCase();

      // 检查文件类型
      if (!fileName.endsWith('.txt') && !fileName.endsWith('.jsonl')) {
        toast.error('仅支持上传 .txt 或 .jsonl 格式的文件');
        return;
      }

      // 检查文件大小 (最大5MB)
      if (file.size > 5 * 1024 * 1024) {
        toast.error('文件大小不能超过 5MB');
        return;
      }

      const reader = new FileReader();
      reader.onload = (e) => {
        const content = e.target?.result as string;
        if (content) {
          // 分析文件内容
          const analysis = analyzeFileContent(content);

          // 如果是二进制文件或包含非 ASCII/Unicode 字符，显示确认对话框
          if (
            analysis.isBinary ||
            analysis.hasNonAscii ||
            analysis.hasNonUnicode
          ) {
            setPendingFile({
              file,
              content,
              analysis,
              currentItems,
              allowDuplicates,
              onSuccess,
            });
            setConfirmationOpen(true);
            return;
          }

          // 直接处理文件
          processFileContent(
            content,
            file,
            currentItems,
            allowDuplicates,
            onSuccess,
          );
        }
      };

      reader.onerror = () => {
        toast.error('文件读取失败');
      };

      reader.readAsText(file, 'UTF-8');
    },
    [processFileContent],
  );

  const openFileUpload = useCallback(() => {
    setFileUploadOpen(true);
  }, []);

  const closeFileUpload = useCallback(() => {
    setFileUploadOpen(false);
  }, []);

  return {
    fileUploadOpen,
    setFileUploadOpen,
    handleFileUpload,
    openFileUpload,
    closeFileUpload,
    confirmationOpen,
    setConfirmationOpen,
    pendingFile,
    handleConfirmUpload,
    handleCancelUpload,
  };
}
