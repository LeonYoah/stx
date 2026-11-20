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
 * Shared table ↔ card item morph: the same item renders in two layouts with
 * matching layoutId anchors, Motion FLIPs them under one tween.
 * 表卡形变共享原语：同一 item 在两种视图里使用相同的 layoutId 锚点，
 * 由 Motion 在统一 tween 下做 FLIP 位移缩放；减少动效时自动退化为瞬时切换。
 */

import {useMemo} from 'react';
import {useReducedMotion, type Transition} from 'motion/react';

/** Morph tween stays in the 150–250ms band. / 形变时长落在 150～250ms。 */
export const VIEW_MORPH_TRANSITION: Transition = {
  type: 'tween',
  duration: 0.2,
  ease: [0.22, 1, 0.36, 1],
};

const VIEW_MORPH_INSTANT: Transition = {duration: 0.01};

export interface ViewMorph {
  /**
   * layoutId for an item part shared across both views (undefined part = the
   * item shell); undefined under reduced motion.
   * item 部件在两种视图间共享的 layoutId（不传 part 表示外壳）；减少动效时为 undefined。
   */
  id: (itemId: string, part?: string) => string | undefined;
  /** Pass to motion `layout` on morph shells. / 传给形变外壳的 motion layout。 */
  layout: boolean;
  transition: Transition;
}

/**
 * Shared-element morph ids + transition for a list that renders the same item
 * in two layouts (table row ↔ card). Both views must reuse the same `part`
 * names so Motion can FLIP between them; wrap the list in LayoutGroup.
 * 供「同一 item 两种布局」列表生成共享锚点 layoutId 与统一过渡；两种视图必须使用
 * 相同的 part 名才能让 Motion 正确对应，列表外层需包 LayoutGroup。
 */
export function useViewMorph(scope: string): ViewMorph {
  const reduceMotion = useReducedMotion() ?? false;
  return useMemo(
    () => ({
      id: (itemId: string, part?: string) =>
        reduceMotion
          ? undefined
          : `${scope}-item-${itemId}${part ? `-${part}` : ''}`,
      layout: !reduceMotion,
      transition: reduceMotion ? VIEW_MORPH_INSTANT : VIEW_MORPH_TRANSITION,
    }),
    [reduceMotion, scope],
  );
}
