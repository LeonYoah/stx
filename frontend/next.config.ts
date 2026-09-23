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

import type {NextConfig} from 'next';

const nextConfig: NextConfig = {
  /* config options here */
  // Generate deployable Node server bundle in .next/standalone
  output: 'standalone',
  // Allow local brand/favicon assets for next/image (Next 16+ requires localPatterns)
  // 允许本地品牌/favicon 资源供 next/image 使用（Next 16+ 需要 localPatterns）
  images: {
    localPatterns: [
      {pathname: '/brand/**'},
      {pathname: '/favicon/**'},
      {pathname: '/icons/**'},
    ],
  },
  experimental: {
    // Allow larger request body for Next rewrite proxy (/api/*), needed by package upload.
    // 允许 Next rewrite 代理处理更大的请求体（/api/*），用于安装包上传。
    middlewareClientMaxBodySize: '1024mb',
  },
  eslint: {
    // 只在构建时跳过 ESLint，开发时照常在编辑器里提示
    ignoreDuringBuilds: true,
  },
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: `${process.env.NEXT_PUBLIC_BACKEND_BASE_URL || 'http://127.0.0.1:17800'}/api/:path*`,
      },
    ];
  },

  // 确保代理请求正确传递headers和cookie
  async headers() {
    return [
      {
        source: '/api/:path*',
        headers: [
          {
            key: 'Access-Control-Allow-Credentials',
            value: 'true',
          },
          {
            key: 'Access-Control-Allow-Origin',
            value:
              process.env.NEXT_PUBLIC_FRONTEND_BASE_URL ||
              'http://38.55.133.202:80',
          },
          {
            key: 'Access-Control-Allow-Methods',
            value: 'GET, POST, PUT, DELETE, OPTIONS',
          },
          {
            key: 'Access-Control-Allow-Headers',
            value: 'Content-Type, Authorization, Cookie',
          },
        ],
      },
    ];
  },
};

export default nextConfig;
