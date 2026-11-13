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

/** Browser event after package/plugin download/upload success / 下载上传成功后的浏览器事件 */
export const ONBOARDING_RESOURCES_CHANGED_EVENT =
  'stx:onboarding-resources-changed';

/**
 * Notify onboarding to re-fetch inventory after download/upload succeeds.
 * 下载/上传成功后通知上手清单重新拉取库存（AC5）。
 */
export function notifyOnboardingResourcesChanged(): void {
  if (typeof window === 'undefined') {
    return;
  }
  window.dispatchEvent(new Event(ONBOARDING_RESOURCES_CHANGED_EVENT));
}
