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

import fs from 'node:fs/promises';
import path from 'node:path';

const frontendDir = process.cwd();
const standaloneDir = path.join(frontendDir, '.next', 'standalone');
const distDir = path.join(frontendDir, 'dist-standalone');

async function pathExists(targetPath) {
  try {
    await fs.access(targetPath);
    return true;
  } catch {
    return false;
  }
}

async function findStandaloneServerEntry(rootDir, maxDepth = 4) {
  const rootEntry = path.join(rootDir, 'server.js');
  if (await pathExists(rootEntry)) {
    return rootEntry;
  }

  async function walk(currentDir, depth) {
    if (depth > maxDepth) {
      return null;
    }

    const entries = await fs.readdir(currentDir, {withFileTypes: true});
    const directories = entries
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .sort();

    const files = entries
      .filter((entry) => entry.isFile() && entry.name === 'server.js')
      .map((entry) => path.join(currentDir, entry.name))
      .sort();

    if (files.length > 0) {
      return files[0];
    }

    for (const directory of directories) {
      const found = await walk(path.join(currentDir, directory), depth + 1);
      if (found) {
        return found;
      }
    }

    return null;
  }

  return walk(rootDir, 1);
}

async function copyOptionalDirectory(sourceDir, targetDir) {
  if (!(await pathExists(sourceDir))) {
    return;
  }

  await fs.mkdir(path.dirname(targetDir), {recursive: true});
  await fs.cp(sourceDir, targetDir, {recursive: true});
}

async function main() {
  if (!(await pathExists(standaloneDir))) {
    throw new Error('未找到 .next/standalone，请先执行 next build');
  }

  const standaloneEntry = await findStandaloneServerEntry(standaloneDir);
  if (!standaloneEntry) {
    throw new Error(
      "未找到 .next/standalone 下的 server.js，请确认 next.config.ts 已配置 output: 'standalone'",
    );
  }

  const entryRelativePath = path.relative(standaloneDir, standaloneEntry);
  const runtimeRelativeDir =
    path.dirname(entryRelativePath) === '.'
      ? ''
      : path.dirname(entryRelativePath);
  const runtimeDir = runtimeRelativeDir
    ? path.join(distDir, runtimeRelativeDir)
    : distDir;

  await fs.rm(distDir, {recursive: true, force: true});
  await fs.mkdir(distDir, {recursive: true});
  await fs.cp(standaloneDir, distDir, {recursive: true});

  // 将 distDir 内指向 standaloneDir 的绝对路径软链接规范化为相对路径，
  // 避免打包到异机解压后因绝对路径不匹配导致 dangling symlink。
  // Normalize absolute symlinks pointing into standaloneDir to relative symlinks,
  // preventing dangling links after extracting on machines with different paths.
  async function normalizeSymlinks(currentDir, standaloneRoot) {
    const entries = await fs.readdir(currentDir, {withFileTypes: true});
    for (const entry of entries) {
      const fullPath = path.join(currentDir, entry.name);
      if (entry.isSymbolicLink()) {
        const target = await fs.readlink(fullPath);
        if (path.isAbsolute(target)) {
          let relSuffix = '';
          if (target.startsWith(standaloneRoot)) {
            relSuffix = path.relative(standaloneRoot, target);
          } else {
            const marker = `${path.sep}.next${path.sep}standalone${path.sep}`;
            const idx = target.indexOf(marker);
            if (idx !== -1) {
              relSuffix = target.slice(idx + marker.length);
            }
          }
          if (relSuffix) {
            const destinationInDist = path.join(distDir, relSuffix);
            const relativeTarget = path.relative(path.dirname(fullPath), destinationInDist);
            await fs.unlink(fullPath);
            await fs.symlink(relativeTarget, fullPath);
          }
        }
      } else if (entry.isDirectory()) {
        await normalizeSymlinks(fullPath, standaloneRoot);
      }
    }
  }
  await normalizeSymlinks(distDir, standaloneDir);

  await copyOptionalDirectory(
    path.join(frontendDir, '.next', 'static'),
    path.join(runtimeDir, '.next', 'static'),
  );
  await copyOptionalDirectory(
    path.join(frontendDir, 'public'),
    path.join(runtimeDir, 'public'),
  );

  console.log(`Packed standalone output: ${entryRelativePath}`);
  console.log(
    `Runtime directory: ${path.relative(frontendDir, runtimeDir) || '.'}`,
  );
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
