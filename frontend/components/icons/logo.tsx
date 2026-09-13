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

/**
 * STX 横向品牌标志。
 * STX horizontal brand lockup.
 */
function STXLogo({className, priority = false}: BrandImageProps) {
  return (
    <Image
      src='/brand/stx-logo.png'
      alt='STX'
      width={560}
      height={160}
      className={className}
      priority={priority}
      draggable={false}
    />
  );
}

/**
 * STX 熊猫图形，用于紧凑位置和小尺寸场景。
 * STX panda mark for compact and small-size placements.
 */
function STXMark({className, priority = false}: BrandImageProps) {
  return (
    <Image
      src='/brand/stx-mark.png'
      alt=''
      width={320}
      height={320}
      className={className}
      priority={priority}
      draggable={false}
    />
  );
}

export {STXLogo, STXMark};
