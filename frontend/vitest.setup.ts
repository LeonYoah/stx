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

import '@testing-library/jest-dom';

// jsdom 没有 IntersectionObserver / ResizeObserver，framer-motion 的 useInView
// （MetricCard 等）与 beUI 图表的尺寸监听需要它们；给个空桩即可。/ jsdom lacks
// IntersectionObserver / ResizeObserver, needed by framer-motion's useInView
// (MetricCard & friends) and beUI chart size watchers — no-op stubs suffice.
class ObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return [];
  }
}
if (typeof globalThis.IntersectionObserver === 'undefined') {
  globalThis.IntersectionObserver =
    ObserverStub as unknown as typeof IntersectionObserver;
}
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver =
    ObserverStub as unknown as typeof ResizeObserver;
}

// jsdom 缺 Pointer Capture / scrollIntoView，Radix Select 开下拉时必调；给空桩。
// jsdom lacks Pointer Capture / scrollIntoView, which Radix Select calls when
// opening; empty stubs suffice.
if (typeof Element !== 'undefined' && !Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
}
if (typeof Element !== 'undefined' && !Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}
