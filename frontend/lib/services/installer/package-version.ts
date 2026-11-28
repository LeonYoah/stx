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

const recommendedVersionPattern =
  /^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z][0-9A-Za-z.-]*)?$/;

const versionInNamePattern =
  /(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z][0-9A-Za-z.-]*)?)/i;

export function isRecommendedSeatunnelVersionFormat(version: string): boolean {
  return recommendedVersionPattern.test(version.trim());
}

export function extractSeatunnelVersionFromPackageFileName(
  fileName: string,
): string {
  const normalized = fileName.trim();
  const official = normalized.match(/apache-seatunnel-(.+?)-bin\.tar\.gz$/i);
  if (official?.[1] && isRecommendedSeatunnelVersionFormat(official[1])) {
    return official[1];
  }
  if (!/seatunnel/i.test(normalized)) {
    return '';
  }
  const stem = normalized.replace(/\.tar\.gz$/i, '').replace(/-bin$/i, '');
  const embedded = stem.match(versionInNamePattern);
  return embedded?.[1] ?? '';
}
