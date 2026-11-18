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

/**
 * 自定义时间窗上限：366 天，与后端 promquery.MaxCustomSpan 一致。所有时间窗控件共用这一处定义，
 * 超出时截取靠近结束的一段并在控件旁提示。
 * Custom window cap: 366 days, matching the backend promquery.MaxCustomSpan. Every time-window
 * control shares this constant; wider picks are trimmed to the part nearest the end with a notice.
 */
export const MAX_CUSTOM_WINDOW_MS = 366 * 24 * 60 * 60 * 1000;
