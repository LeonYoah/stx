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

import Image from 'next/image';

interface BrandImageProps {
  className?: string;
  priority?: boolean;
}

/** 资源版本号，用于刷新浏览器与 Next Image 缓存。 / Asset version for cache busting. */
const BRAND_ASSET_VERSION = 'qingluan-chosen-transparent-1';

/**
 * STX 浅色横向锁章（青鸾鸟标 + STX 字标），透明底。
 * STX light lockup (Qingluan mark + STX wordmark), transparent background.
 */
function STXLogo({className, priority = false}: BrandImageProps) {
  return (
    <Image
      src={`/brand/stx-logo.png?v=${BRAND_ASSET_VERSION}`}
      alt='STX'
      width={752}
      height={390}
      className={className}
      priority={priority}
      draggable={false}
    />
  );
}

/**
 * STX 深色横向锁章（青鸾鸟标 + 白字 STX），透明底，可叠任意深色容器。
 * STX dark lockup (Qingluan mark + white STX), transparent for any dark surface.
 */
function STXLogoDark({className, priority = false}: BrandImageProps) {
  return (
    <Image
      src={`/brand/stx-logo-dark.png?v=${BRAND_ASSET_VERSION}`}
      alt='STX'
      width={752}
      height={391}
      className={className}
      priority={priority}
      draggable={false}
    />
  );
}

/**
 * STX 青鸾图形标，用于侧栏、favicon 级小尺寸与仅需图标的场景。
 * STX Qingluan mark for compact placements (sidebar, favicon-scale, icon-only).
 */
function STXMark({className, priority = false}: BrandImageProps) {
  return (
    <Image
      src={`/brand/stx-mark.png?v=${BRAND_ASSET_VERSION}`}
      alt=''
      width={866}
      height={655}
      className={className}
      priority={priority}
      draggable={false}
    />
  );
}

export {STXLogo, STXLogoDark, STXMark};
