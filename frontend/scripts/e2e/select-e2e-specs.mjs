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

import {appendFileSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import path from 'node:path';
import process from 'node:process';
import {fileURLToPath} from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '../../..');

const allSpecs = [
  'e2e/login-ui.spec.ts',
  'e2e/dashboard.spec.ts',
  'e2e/plugin-dependency-template.spec.ts',
  'e2e/install-wizard-template.spec.ts',
  'e2e/install-wizard-negative.spec.ts',
  'e2e/upgrade-prepare-template.spec.ts',
  'e2e/upgrade-prepare-negative.spec.ts',
];

const smokeExcludedSpecs = new Set([
  'e2e/install-wizard-real.spec.ts',
  'e2e/stx-cli-installer-real.spec.ts',
  'e2e/workbench-real.spec.ts',
  'e2e/config-real.spec.ts',
  'e2e/upgrade-real.spec.ts',
  'e2e/plugin-real.spec.ts',
]);

const globalPatterns = [
  /^frontend\/playwright\.config\.ts$/,
  /^frontend\/package\.json$/,
  /^frontend\/pnpm-lock\.yaml$/,
  /^frontend\/e2e\/auth\.setup\.ts$/,
  /^frontend\/scripts\/e2e\/mock-api-server\.mjs$/,
  /^frontend\/scripts\/e2e\/select-e2e-specs\.mjs$/,
  /^frontend\/components\/ui\//,
  /^frontend\/lib\/i18n\//,
  /^frontend\/app\/layout\.tsx$/,
  /^frontend\/app\/\(main\)\/layout\.tsx$/,
  /^config\.e2e\.yaml$/,
  /^\.github\/workflows\/ci-main\.yml$/,
];

const groups = [
  {
    name: 'plugin',
    patterns: [
      /^frontend\/components\/common\/plugin\//,
      /^frontend\/hooks\/use-plugin\.ts$/,
      /^frontend\/lib\/services\/plugin\//,
      /^frontend\/app\/\(main\)\/plugins\//,
      /^frontend\/e2e\/plugin-dependency-template\.spec\.ts$/,
      /^frontend\/e2e\/helpers\/plugin-dependency-template\.ts$/,
    ],
    specs: ['e2e/plugin-dependency-template.spec.ts'],
  },
  {
    name: 'installer',
    patterns: [
      /^frontend\/components\/common\/installer\//,
      /^frontend\/hooks\/use-installer\.ts$/,
      /^frontend\/lib\/services\/installer\//,
      /^frontend\/app\/\(main\)\/e2e-lab\/install-wizard\//,
      /^frontend\/e2e\/install-wizard-template\.spec\.ts$/,
      /^frontend\/e2e\/install-wizard-negative\.spec\.ts$/,
      /^frontend\/e2e\/helpers\/install-wizard-template\.ts$/,
    ],
    specs: [
      'e2e/install-wizard-template.spec.ts',
      'e2e/install-wizard-negative.spec.ts',
    ],
  },
  {
    name: 'upgrade',
    patterns: [
      /^frontend\/components\/common\/cluster\/upgrade\//,
      /^frontend\/lib\/services\/st-upgrade\//,
      /^frontend\/lib\/st-upgrade-session\.ts$/,
      /^frontend\/app\/\(main\)\/clusters\/.*\/upgrade\//,
      /^frontend\/e2e\/upgrade-prepare-template\.spec\.ts$/,
      /^frontend\/e2e\/upgrade-prepare-negative\.spec\.ts$/,
      /^frontend\/e2e\/helpers\/upgrade-prepare-template\.ts$/,
    ],
    specs: [
      'e2e/upgrade-prepare-template.spec.ts',
      'e2e/upgrade-prepare-negative.spec.ts',
    ],
  },
  {
    name: 'dashboard',
    patterns: [
      /^frontend\/components\/common\/dashboard\//,
      /^frontend\/app\/\(main\)\/dashboard\//,
      /^frontend\/lib\/services\/dashboard\//,
      /^frontend\/e2e\/dashboard\.spec\.ts$/,
    ],
    specs: ['e2e/dashboard.spec.ts'],
  },
  {
    name: 'auth',
    patterns: [
      /^frontend\/app\/\(auth\)\//,
      /^frontend\/components\/common\/auth\//,
      /^frontend\/lib\/services\/auth\//,
      /^frontend\/middleware\.ts$/,
      /^frontend\/e2e\/login-ui\.spec\.ts$/,
    ],
    specs: ['e2e/login-ui.spec.ts', 'e2e/dashboard.spec.ts'],
  },
];

function parseArgs(argv) {
  const parsed = {
    all: false,
    base: '',
    head: 'HEAD',
    githubOutput: '',
    changedFiles: [],
  };

  for (let index = 0; index < argv.length; index += 1) {
    const current = argv[index];
    const next = argv[index + 1];

    if (current === '--base' && next) {
      parsed.base = next;
      index += 1;
      continue;
    }
    if (current === '--all') {
      parsed.all = true;
      continue;
    }
    if (current === '--head' && next) {
      parsed.head = next;
      index += 1;
      continue;
    }
    if (current === '--github-output' && next) {
      parsed.githubOutput = next;
      index += 1;
      continue;
    }
    if (current === '--changed-file' && next) {
      parsed.changedFiles.push(next);
      index += 1;
    }
  }

  return parsed;
}

function readChangedFiles(base, head) {
  const output = execFileSync(
    'git',
    ['diff', '--name-only', '--diff-filter=ACDMR', base, head],
    {
      cwd: repoRoot,
      encoding: 'utf8',
    },
  );

  return output
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean);
}

function matchesAnyPattern(file, patterns) {
  return patterns.some((pattern) => pattern.test(file));
}

function normalizeExplicitSpecs(changedFiles) {
  return changedFiles
    .filter((file) => /^frontend\/e2e\/.+\.spec\.ts$/.test(file))
    .map((file) => file.replace(/^frontend\//, ''))
    .filter((file) => !smokeExcludedSpecs.has(file));
}

function unique(items) {
  return [...new Set(items)];
}

function selectSpecs(changedFiles) {
  const explicitSpecs = normalizeExplicitSpecs(changedFiles);

  if (changedFiles.some((file) => matchesAnyPattern(file, globalPatterns))) {
    return {
      mode: 'all',
      specs: unique([...allSpecs, ...explicitSpecs]),
      matchedGroups: ['global'],
    };
  }

  const matchedGroups = groups
    .filter((group) =>
      changedFiles.some((file) => matchesAnyPattern(file, group.patterns)),
    )
    .map((group) => group.name);

  const specs = unique([
    ...explicitSpecs,
    ...groups
      .filter((group) => matchedGroups.includes(group.name))
      .flatMap((group) => group.specs),
  ]);

  return {
    mode: specs.length > 0 ? 'selected' : 'none',
    specs,
    matchedGroups,
  };
}

function writeGithubOutputs(outputPath, selection) {
  const lines = [
    `mode=${selection.mode}`,
    `has_specs=${selection.specs.length > 0 ? 'true' : 'false'}`,
    `spec_count=${selection.specs.length}`,
    `matched_groups=${selection.matchedGroups.join(',')}`,
    `spec_args=${selection.specs.join(' ')}`,
  ];
  appendFileSync(outputPath, `${lines.join('\n')}\n`, 'utf8');
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  if (!args.all && args.changedFiles.length === 0 && !args.base) {
    throw new Error(
      'Either --all, --base, or one or more --changed-file arguments is required.',
    );
  }
  const changedFiles = args.all
    ? []
    : args.changedFiles.length > 0
      ? args.changedFiles
      : readChangedFiles(args.base, args.head);
  const selection = args.all
    ? {
        mode: 'all',
        specs: unique([...allSpecs]),
        matchedGroups: ['global'],
      }
    : selectSpecs(changedFiles);

  const payload = {
    ...selection,
    changedFiles,
  };

  if (args.githubOutput) {
    writeGithubOutputs(args.githubOutput, selection);
  }

  process.stdout.write(`${JSON.stringify(payload, null, 2)}\n`);
}

main();
