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
 * Firing-alert count for the workspace tab badge (stats.total only, page_size=1).
 * 工作台标签上的触发中计数（只取 stats，page_size=1 省流量）。
 */
import {useEffect, useState} from 'react';
import services from '@/lib/services';

export function useMonitoringFiringCount(): number {
  const [count, setCount] = useState(0);
  useEffect(() => {
    let alive = true;
    services.monitoring
      .getAlertInstances({status: 'firing', page: 1, page_size: 1})
      .then((data) => {
        if (alive) {
          setCount(data?.stats?.firing ?? data?.total ?? 0);
        }
      })
      .catch(() => {
        if (alive) {
          setCount(0);
        }
      });
    return () => {
      alive = false;
    };
  }, []);
  return count;
}
