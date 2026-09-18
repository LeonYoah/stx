'use client';

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

import {useEffect} from 'react';
import {ensureKapaWidget, startKapaThemeSync} from '@/lib/services/kapa-ai';

/**
 * Permanently mount the Kapa Ask AI floating widget on the main app shell.
 * 在主应用壳层永久挂载 Kapa Ask AI 右下角悬浮按钮（进入控制台即可见，无需先点入口）。
 */
export function KapaWidgetMount() {
  useEffect(() => {
    void ensureKapaWidget().then(() => {
      startKapaThemeSync();
    });
  }, []);

  return null;
}
