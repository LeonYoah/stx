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

import http from 'node:http';

const port = Number(process.env.MOCK_API_PORT ?? 8010);
const sessionCookieName = 'stx_mock_session';
const sessionCookieValue = 'mock-admin';

function json(res, statusCode, payload, extraHeaders = {}) {
  res.writeHead(statusCode, {
    'Content-Type': 'application/json; charset=utf-8',
    ...extraHeaders,
  });
  res.end(JSON.stringify(payload));
}

function html(res, statusCode, body) {
  res.writeHead(statusCode, {
    'Content-Type': 'text/html; charset=utf-8',
  });
  res.end(body);
}

function parseCookies(cookieHeader = '') {
  return cookieHeader
    .split(';')
    .map((item) => item.trim())
    .filter(Boolean)
    .reduce((result, pair) => {
      const separatorIndex = pair.indexOf('=');
      if (separatorIndex === -1) {
        return result;
      }
      const key = pair.slice(0, separatorIndex);
      const value = pair.slice(separatorIndex + 1);
      result[key] = value;
      return result;
    }, {});
}

function hasValidSession(req) {
  const cookies = parseCookies(req.headers.cookie);
  return cookies[sessionCookieName] === sessionCookieValue;
}

function readJsonBody(req) {
  return new Promise((resolve, reject) => {
    let body = '';
    req.on('data', (chunk) => {
      body += chunk;
    });
    req.on('end', () => {
      if (!body) {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(body));
      } catch (error) {
        reject(error);
      }
    });
    req.on('error', reject);
  });
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url ?? '/', `http://127.0.0.1:${port}`);
  const {pathname} = url;

  if (pathname === '/api/v1/health') {
    json(res, 200, {error_msg: '', data: {status: 'ok'}});
    return;
  }

  if (pathname === '/api/v1/auth/login' && req.method === 'POST') {
    const payload = await readJsonBody(req).catch(() => null);
    if (
      payload?.username !== (process.env.E2E_USERNAME ?? 'admin') ||
      payload?.password !== (process.env.E2E_PASSWORD ?? 'admin123')
    ) {
      json(res, 401, {error_msg: '用户名或密码错误', data: null});
      return;
    }

    json(
      res,
      200,
      {
        error_msg: '',
        data: {
          id: 1,
          username: process.env.E2E_USERNAME ?? 'admin',
          nickname: '系统管理员',
          email: '',
          avatar: '',
          is_admin: true,
          language: 'zh',
        },
      },
      {
        'Set-Cookie': `${sessionCookieName}=${sessionCookieValue}; Path=/; HttpOnly; SameSite=Lax`,
      },
    );
    return;
  }

  if (pathname === '/api/v1/auth/logout' && req.method === 'POST') {
    json(
      res,
      200,
      {error_msg: '', data: null},
      {
        'Set-Cookie': `${sessionCookieName}=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax`,
      },
    );
    return;
  }

  if (pathname === '/api/v1/auth/user-info') {
    if (!hasValidSession(req)) {
      json(res, 401, {error_msg: '未登录', data: null});
      return;
    }

    json(res, 200, {
      error_msg: '',
      data: {
        id: 1,
        username: process.env.E2E_USERNAME ?? 'admin',
        nickname: '系统管理员',
        email: '',
        avatar: '',
        is_admin: true,
        language: 'zh',
      },
    });
    return;
  }

  if (pathname === '/api/v1/dashboard/overview') {
    if (!hasValidSession(req)) {
      json(res, 401, {error_msg: '未登录', data: null});
      return;
    }

    json(res, 200, {
      error_msg: '',
      data: {
        generated_at: new Date().toISOString(),
        stats: {
          total_hosts: 0,
          online_hosts: 0,
          total_clusters: 0,
          running_clusters: 0,
          total_nodes: 0,
          running_nodes: 0,
          total_agents: 0,
          online_agents: 0,
        },
        clusters: [],
        hosts: [],
        recent_activities: [],
      },
    });
    return;
  }

  if (pathname === '/api/v1/monitoring/platform-health') {
    if (!hasValidSession(req)) {
      json(res, 401, {error_msg: '未登录', data: null});
      return;
    }

    json(res, 200, {
      error_msg: '',
      data: {
        generated_at: new Date().toISOString(),
        health_status: 'unknown',
        total_clusters: 0,
        healthy_clusters: 0,
        degraded_clusters: 0,
        unhealthy_clusters: 0,
        unknown_clusters: 0,
        active_alerts: 0,
        critical_alerts: 0,
      },
    });
    return;
  }

  if (pathname === '/api/v1/clusters/health') {
    if (!hasValidSession(req)) {
      json(res, 401, {error_msg: '未登录', data: null});
      return;
    }

    json(res, 200, {
      error_msg: '',
      data: {
        generated_at: new Date().toISOString(),
        total: 0,
        clusters: [],
      },
    });
    return;
  }

  if (pathname.startsWith('/api/v1/monitoring/proxy/grafana/')) {
    html(
      res,
      200,
      '<!doctype html><html><body><main>Mock Grafana Dashboard</main></body></html>',
    );
    return;
  }

  json(res, 404, {error_msg: `Unhandled mock route: ${pathname}`, data: null});
});

server.listen(port, '127.0.0.1', () => {
  console.log(`[mock-api] listening on http://127.0.0.1:${port}`);
});
