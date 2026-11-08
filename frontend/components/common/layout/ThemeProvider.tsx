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

import * as React from 'react';
import {ThemeProvider as NextThemesProvider, useTheme} from 'next-themes';

/**
 * 把 next-themes 的解析结果写到 html[data-theme]，Arc 的 CSS Module 只认这个属性。
 * Mirror the resolved next-themes value onto html[data-theme]; Arc CSS modules read that attribute.
 */
function ArcThemeAttribute() {
  const {resolvedTheme} = useTheme();
  React.useEffect(() => {
    document.documentElement.dataset.theme =
      resolvedTheme === 'light' ? 'light' : 'dark';
  }, [resolvedTheme]);
  return null;
}

export function ThemeProvider({
  children,
  ...props
}: React.ComponentProps<typeof NextThemesProvider>) {
  return (
    <NextThemesProvider {...props}>
      <ArcThemeAttribute />
      {children}
    </NextThemesProvider>
  );
}
