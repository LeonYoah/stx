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

package io.github.leonyoah.stx.proxy.service.config;

import org.apache.seatunnel.shade.com.typesafe.config.Config;
import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.lang.reflect.InvocationTargetException;
import java.net.HttpURLConnection;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.URI;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;
import java.util.Properties;
import java.util.TimeZone;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * 协议级连接探针：Kafka (AdminClient describeCluster)、S3 (SigV4 ListObjectsV2)、FTP (USER/PASS 登录)、SFTP
 * (JSch 认证)。与只做 TCP 握手不同，这些探针会真正校验认证信息。第三方客户端库（kafka-clients、JSch）均通过插件类加载器反射加载， 与生产
 * SEATUNNEL_HOME/connectors 中连接器自带的版本一致；S3 与 FTP 使用 JDK 原生实现，无额外依赖。
 *
 * <p>Protocol-level connection probes that really verify credentials instead of a TCP handshake.
 * Third-party clients (kafka-clients, JSch) are loaded reflectively from the plugin classloader
 * (the versions bundled with the connectors under SEATUNNEL_HOME); S3 and FTP use plain JDK code.
 */
final class ConnectionProtocolProbes {

    private ConnectionProtocolProbes() {}

    /**
     * 针对已知连接器执行协议级探测；不适用时返回 null。 Runs a protocol-level probe for known connectors; returns null
     * when not applicable.
     */
    static Map<String, Object> probe(
            String pluginName, Config config, ClassLoader classLoader, int timeoutMillis) {
        if (pluginName == null) {
            return null;
        }
        switch (pluginName.toLowerCase(Locale.ROOT)) {
            case "kafka":
                return probeKafka(config, classLoader, timeoutMillis);
            case "s3file":
                return probeS3(config, timeoutMillis);
            case "ftpfile":
                return probeFtp(config, timeoutMillis);
            case "sftpfile":
                return probeSftp(config, classLoader, timeoutMillis);
            default:
                return null;
        }
    }

    // ------------------------------------------------------------------ Kafka

    static Map<String, Object> probeKafka(
            Config config, ClassLoader classLoader, int timeoutMillis) {
        String method = "kafka-admin";
        String bootstrap = ConnectionProbeSupport.getString(config, "bootstrap.servers");
        if (StringUtils.isBlank(bootstrap)) {
            return null;
        }
        Class<?> adminClass;
        try {
            adminClass =
                    Class.forName("org.apache.kafka.clients.admin.AdminClient", true, classLoader);
        } catch (Throwable e) {
            return unverified(method, "未找到 kafka-clients（Kafka 连接器未安装？），认证信息未验证。");
        }
        Properties props = new Properties();
        props.putAll(kafkaClientProperties(config));
        props.put("bootstrap.servers", normalizeBootstrap(bootstrap));
        int budget = Math.max(500, timeoutMillis - 300);
        props.put("request.timeout.ms", String.valueOf(budget));
        props.put("default.api.timeout.ms", String.valueOf(budget));
        props.put("socket.connection.setup.timeout.ms", String.valueOf(Math.min(budget, 1500)));
        props.put("socket.connection.setup.timeout.max.ms", String.valueOf(Math.min(budget, 1500)));
        props.put("reconnect.backoff.max.ms", "500");
        props.put("retries", "0");
        props.put("client.id", "stx-connection-probe");
        Object admin = null;
        try {
            admin = adminClass.getMethod("create", Properties.class).invoke(null, props);
            Object describe = adminClass.getMethod("describeCluster").invoke(admin);
            Class<?> resultClass =
                    Class.forName(
                            "org.apache.kafka.clients.admin.DescribeClusterResult",
                            false,
                            adminClass.getClassLoader());
            Object nodesFuture = resultClass.getMethod("nodes").invoke(describe);
            Class<?> kafkaFuture =
                    Class.forName(
                            "org.apache.kafka.common.KafkaFuture",
                            false,
                            adminClass.getClassLoader());
            Object nodes =
                    kafkaFuture
                            .getMethod("get", long.class, TimeUnit.class)
                            .invoke(nodesFuture, (long) budget, TimeUnit.MILLISECONDS);
            int count =
                    nodes instanceof java.util.Collection
                            ? ((java.util.Collection<?>) nodes).size()
                            : -1;
            Map<String, Object> res = new LinkedHashMap<>();
            res.put("status", "success");
            res.put("probeMethod", method);
            res.put(
                    "message",
                    "Kafka cluster metadata fetched via AdminClient"
                            + (count >= 0 ? " (" + count + " broker(s))." : "."));
            return res;
        } catch (Throwable t) {
            return failed(method, unwrap(t));
        } finally {
            closeKafkaAdmin(adminClass, admin);
        }
    }

    private static void closeKafkaAdmin(Class<?> adminClass, Object admin) {
        if (admin == null) {
            return;
        }
        try {
            adminClass
                    .getMethod("close", java.time.Duration.class)
                    .invoke(admin, java.time.Duration.ofMillis(500));
            return;
        } catch (Throwable ignored) {
            // try older signature
        }
        try {
            adminClass
                    .getMethod("close", long.class, TimeUnit.class)
                    .invoke(admin, 500L, TimeUnit.MILLISECONDS);
        } catch (Throwable ignored) {
            // no-op
        }
    }

    /**
     * 读取 kafka.config 并扁平化为 Kafka 客户端属性。兼容 kafka.config = {...} 对象写法与 "kafka.config.xxx" 字面量 key
     * 写法（SeaTunnel 的 HOCON 解析不会按点号拆分 key）。 Reads kafka.config as client properties, supporting both
     * the object form and literal "kafka.config.xxx" keys (SeaTunnel's HOCON keeps dotted keys
     * literal).
     */
    @SuppressWarnings("unchecked")
    static Map<String, String> kafkaClientProperties(Config config) {
        Map<String, String> result = new LinkedHashMap<>();
        Map<String, Object> root;
        try {
            root = config.root().unwrapped();
        } catch (Exception e) {
            return result;
        }
        String prefix = "kafka.config";
        for (Map.Entry<String, Object> e : root.entrySet()) {
            String key = e.getKey();
            Object value = e.getValue();
            if (prefix.equals(key)) {
                if (value instanceof Map) {
                    flattenMap("", (Map<String, Object>) value, result);
                }
            } else if (key.startsWith(prefix + ".")) {
                String sub = key.substring(prefix.length() + 1);
                if (value instanceof Map) {
                    flattenMap(sub, (Map<String, Object>) value, result);
                } else if (value != null) {
                    result.put(sub, String.valueOf(value));
                }
            } else if ("kafka".equals(key) && value instanceof Map) {
                Object nested = ((Map<String, Object>) value).get("config");
                if (nested instanceof Map) {
                    flattenMap("", (Map<String, Object>) nested, result);
                }
            }
        }
        return result;
    }

    @SuppressWarnings("unchecked")
    private static void flattenMap(
            String prefix, Map<String, Object> map, Map<String, String> out) {
        for (Map.Entry<String, Object> e : map.entrySet()) {
            String key = StringUtils.isBlank(prefix) ? e.getKey() : prefix + "." + e.getKey();
            Object v = e.getValue();
            if (v instanceof Map) {
                flattenMap(key, (Map<String, Object>) v, out);
            } else if (v != null) {
                out.put(key, String.valueOf(v));
            }
        }
    }

    /** 未写端口的 broker 补全 9092。 Adds the default 9092 port to brokers without one. */
    static String normalizeBootstrap(String bootstrap) {
        StringBuilder sb = new StringBuilder();
        for (String raw : bootstrap.split(",")) {
            String server = raw.trim();
            if (server.isEmpty()) {
                continue;
            }
            int scheme = server.indexOf("://");
            String hostPort = scheme > 0 ? server.substring(scheme + 3) : server;
            boolean hasPort =
                    hostPort.startsWith("[")
                            ? hostPort.contains("]:")
                            : hostPort.lastIndexOf(':') > 0
                                    && hostPort.indexOf(':') == hostPort.lastIndexOf(':');
            if (sb.length() > 0) {
                sb.append(',');
            }
            sb.append(server);
            if (!hasPort) {
                sb.append(":9092");
            }
        }
        return sb.toString();
    }

    // --------------------------------------------------------------------- S3

    static Map<String, Object> probeS3(Config config, int timeoutMillis) {
        String method = "s3-api";
        String bucketOption = ConnectionProbeSupport.getString(config, "bucket");
        String bucket = parseBucket(bucketOption);
        if (StringUtils.isBlank(bucket)) {
            return null;
        }
        String accessKey = firstNonBlank(config, "access_key", "fs.s3a.access.key");
        String secretKey = firstNonBlank(config, "secret_key", "fs.s3a.secret.key");
        if (StringUtils.isBlank(accessKey) || StringUtils.isBlank(secretKey)) {
            return unverified(method, "未配置 access_key/secret_key（可能使用实例角色等凭据提供者），无法在线验证 S3 认证信息。");
        }
        String sessionToken = firstNonBlank(config, "session_token", "fs.s3a.session.token");
        String endpoint = ConnectionProbeSupport.resolveS3Endpoint(config);
        String region = firstNonBlank(config, "region", "fs.s3a.endpoint.region", "s3.region");
        if (StringUtils.isBlank(region)) {
            region = regionFromEndpoint(endpoint);
        }
        long deadline = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(timeoutMillis);
        try {
            S3Response response =
                    s3ListObjects(
                            endpoint, bucket, region, accessKey, secretKey, sessionToken, deadline);
            if ((response.code == 301 || response.code == 400)
                    && StringUtils.isNotBlank(response.bucketRegion)
                    && !response.bucketRegion.equals(region)) {
                // 存储桶在其他 Region：按服务端提示的 Region 重试一次。
                // Bucket lives in another region: retry once with the region the server reported.
                String retryEndpoint =
                        StringUtils.isBlank(endpoint) || isAwsEndpoint(endpoint) ? null : endpoint;
                response =
                        s3ListObjects(
                                retryEndpoint,
                                bucket,
                                response.bucketRegion,
                                accessKey,
                                secretKey,
                                sessionToken,
                                deadline);
            }
            Map<String, Object> res = new LinkedHashMap<>();
            res.put("probeMethod", method);
            if (response.code == 200) {
                res.put("status", "success");
                res.put(
                        "message",
                        "S3 credentials verified; bucket '" + bucket + "' is accessible.");
                return res;
            }
            res.put("status", "failed");
            String detail =
                    String.format(
                            "HTTP %d %s%s",
                            response.code,
                            StringUtils.defaultString(response.errorCode),
                            StringUtils.isNotBlank(response.errorMessage)
                                    ? ": " + response.errorMessage
                                    : "");
            if (response.code == 403
                    || "InvalidAccessKeyId".equals(response.errorCode)
                    || "SignatureDoesNotMatch".equals(response.errorCode)) {
                res.put("message", "认证失败 (authentication failed): S3 " + detail.trim());
            } else if ("NoSuchBucket".equals(response.errorCode) || response.code == 404) {
                res.put("message", "S3 bucket '" + bucket + "' 不存在 (" + detail.trim() + ")");
            } else {
                res.put("message", "S3 请求失败: " + detail.trim());
            }
            return res;
        } catch (Throwable t) {
            return failed(method, t);
        }
    }

    static final class S3Response {
        int code;
        String errorCode;
        String errorMessage;
        String bucketRegion;
    }

    private static final Pattern XML_CODE = Pattern.compile("<Code>([^<]*)</Code>");
    private static final Pattern XML_MESSAGE = Pattern.compile("<Message>([^<]*)</Message>");
    private static final Pattern XML_REGION = Pattern.compile("<Region>([^<]*)</Region>");
    private static final String EMPTY_SHA256 =
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855";

    /** 以 SigV4 签名发起 ListObjectsV2(max-keys=1)。 Issues a SigV4-signed ListObjectsV2 (max-keys=1). */
    static S3Response s3ListObjects(
            String endpoint,
            String bucket,
            String region,
            String accessKey,
            String secretKey,
            String sessionToken,
            long deadlineNanos)
            throws Exception {
        return s3Request(
                "GET",
                "list-type=2&max-keys=1",
                endpoint,
                bucket,
                region,
                accessKey,
                secretKey,
                sessionToken,
                deadlineNanos);
    }

    /** 以 SigV4 签名发起无请求体的 bucket 级请求。 Issues a SigV4-signed bucket-level request without body. */
    static S3Response s3Request(
            String httpMethod,
            String query,
            String endpoint,
            String bucket,
            String region,
            String accessKey,
            String secretKey,
            String sessionToken,
            long deadlineNanos)
            throws Exception {
        String effectiveRegion = StringUtils.defaultIfBlank(region, "us-east-1");
        String scheme;
        String hostPort;
        String canonicalPath;
        if (StringUtils.isBlank(endpoint)) {
            scheme = "https";
            hostPort =
                    bucket.contains(".")
                            ? "s3." + effectiveRegion + ".amazonaws.com"
                            : bucket + ".s3." + effectiveRegion + ".amazonaws.com";
            canonicalPath = bucket.contains(".") ? "/" + bucket : "/";
        } else {
            String ep = endpoint.trim();
            if (!ep.contains("://")) {
                ep = "https://" + ep;
            }
            URI uri = URI.create(ep);
            scheme = uri.getScheme().toLowerCase(Locale.ROOT);
            int port = uri.getPort();
            boolean defaultPort =
                    port == -1
                            || ("http".equals(scheme) && port == 80)
                            || ("https".equals(scheme) && port == 443);
            hostPort = uri.getHost() + (defaultPort ? "" : ":" + port);
            // 自定义 Endpoint（MinIO 等）统一使用 path-style。 Custom endpoints use path-style.
            canonicalPath = "/" + bucket;
        }
        URL url =
                new URL(
                        scheme
                                + "://"
                                + hostPort
                                + canonicalPath
                                + (StringUtils.isBlank(query) ? "" : "?" + query));

        SimpleDateFormat amzFormat = new SimpleDateFormat("yyyyMMdd'T'HHmmss'Z'", Locale.ROOT);
        amzFormat.setTimeZone(TimeZone.getTimeZone("UTC"));
        String amzDate = amzFormat.format(new Date());
        String dateStamp = amzDate.substring(0, 8);

        StringBuilder canonicalHeaders = new StringBuilder();
        canonicalHeaders.append("host:").append(hostPort).append('\n');
        canonicalHeaders.append("x-amz-content-sha256:").append(EMPTY_SHA256).append('\n');
        canonicalHeaders.append("x-amz-date:").append(amzDate).append('\n');
        String signedHeaders = "host;x-amz-content-sha256;x-amz-date";
        if (StringUtils.isNotBlank(sessionToken)) {
            canonicalHeaders.append("x-amz-security-token:").append(sessionToken).append('\n');
            signedHeaders += ";x-amz-security-token";
        }
        String canonicalRequest =
                httpMethod
                        + "\n"
                        + canonicalPath
                        + "\n"
                        + StringUtils.defaultString(query)
                        + "\n"
                        + canonicalHeaders
                        + "\n"
                        + signedHeaders
                        + "\n"
                        + EMPTY_SHA256;
        String scope = dateStamp + "/" + effectiveRegion + "/s3/aws4_request";
        String stringToSign =
                "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex(canonicalRequest);
        byte[] signingKey =
                hmac(
                        hmac(
                                hmac(
                                        hmac(
                                                ("AWS4" + secretKey)
                                                        .getBytes(StandardCharsets.UTF_8),
                                                dateStamp),
                                        effectiveRegion),
                                "s3"),
                        "aws4_request");
        String signature = hex(hmac(signingKey, stringToSign));
        String authorization =
                "AWS4-HMAC-SHA256 Credential="
                        + accessKey
                        + "/"
                        + scope
                        + ", SignedHeaders="
                        + signedHeaders
                        + ", Signature="
                        + signature;

        HttpURLConnection conn = (HttpURLConnection) url.openConnection();
        try {
            int remaining =
                    (int)
                            Math.max(
                                    500,
                                    TimeUnit.NANOSECONDS.toMillis(
                                            deadlineNanos - System.nanoTime()));
            conn.setConnectTimeout(remaining);
            conn.setReadTimeout(remaining);
            conn.setInstanceFollowRedirects(false);
            conn.setUseCaches(false);
            conn.setRequestMethod(httpMethod);
            conn.setRequestProperty("x-amz-date", amzDate);
            conn.setRequestProperty("x-amz-content-sha256", EMPTY_SHA256);
            if (StringUtils.isNotBlank(sessionToken)) {
                conn.setRequestProperty("x-amz-security-token", sessionToken);
            }
            conn.setRequestProperty("Authorization", authorization);
            if ("PUT".equals(httpMethod) || "POST".equals(httpMethod)) {
                conn.setDoOutput(true);
                conn.setFixedLengthStreamingMode(0);
                conn.getOutputStream().close();
            }
            S3Response response = new S3Response();
            response.code = conn.getResponseCode();
            response.bucketRegion = conn.getHeaderField("x-amz-bucket-region");
            InputStream body = response.code >= 300 ? conn.getErrorStream() : conn.getInputStream();
            String xml = readLimited(body, 16 * 1024);
            if (xml != null) {
                Matcher code = XML_CODE.matcher(xml);
                if (code.find()) {
                    response.errorCode = code.group(1);
                }
                Matcher message = XML_MESSAGE.matcher(xml);
                if (message.find()) {
                    response.errorMessage = message.group(1);
                }
                if (StringUtils.isBlank(response.bucketRegion)) {
                    Matcher reg = XML_REGION.matcher(xml);
                    if (reg.find()) {
                        response.bucketRegion = reg.group(1);
                    }
                }
            }
            return response;
        } finally {
            conn.disconnect();
        }
    }

    static String parseBucket(String bucketOption) {
        if (StringUtils.isBlank(bucketOption)) {
            return null;
        }
        String b = bucketOption.trim();
        int scheme = b.indexOf("://");
        if (scheme >= 0) {
            b = b.substring(scheme + 3);
        }
        while (b.startsWith("/")) {
            b = b.substring(1);
        }
        int slash = b.indexOf('/');
        if (slash >= 0) {
            b = b.substring(0, slash);
        }
        return StringUtils.trimToNull(b);
    }

    private static boolean isAwsEndpoint(String endpoint) {
        return endpoint != null && endpoint.toLowerCase(Locale.ROOT).contains("amazonaws.com");
    }

    private static final Pattern AWS_REGION =
            Pattern.compile("(?i)s3[.-]([a-z0-9-]+)\\.amazonaws\\.com");

    static String regionFromEndpoint(String endpoint) {
        if (StringUtils.isBlank(endpoint)) {
            return "us-east-1";
        }
        Matcher m = AWS_REGION.matcher(endpoint);
        if (m.find() && !"external-1".equalsIgnoreCase(m.group(1))) {
            return m.group(1).toLowerCase(Locale.ROOT);
        }
        return "us-east-1";
    }

    private static String readLimited(InputStream in, int limit) {
        if (in == null) {
            return null;
        }
        try (InputStream input = in) {
            byte[] buf = new byte[limit];
            int total = 0;
            int n;
            while (total < limit && (n = input.read(buf, total, limit - total)) > 0) {
                total += n;
            }
            return new String(buf, 0, total, StandardCharsets.UTF_8);
        } catch (IOException e) {
            return null;
        }
    }

    private static byte[] hmac(byte[] key, String data) throws Exception {
        Mac mac = Mac.getInstance("HmacSHA256");
        mac.init(new SecretKeySpec(key, "HmacSHA256"));
        return mac.doFinal(data.getBytes(StandardCharsets.UTF_8));
    }

    private static String sha256Hex(String data) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        return hex(digest.digest(data.getBytes(StandardCharsets.UTF_8)));
    }

    private static String hex(byte[] bytes) {
        StringBuilder sb = new StringBuilder(bytes.length * 2);
        for (byte b : bytes) {
            sb.append(String.format("%02x", b & 0xff));
        }
        return sb.toString();
    }

    // -------------------------------------------------------------------- FTP

    static Map<String, Object> probeFtp(Config config, int timeoutMillis) {
        String method = "ftp-login";
        String host = ConnectionProbeSupport.getString(config, "host");
        if (StringUtils.isBlank(host)) {
            return null;
        }
        int port = parsePort(ConnectionProbeSupport.getString(config, "port"), 21);
        String user = firstNonBlank(config, "user", "username");
        String password = ConnectionProbeSupport.getString(config, "password");
        if (StringUtils.isBlank(user)) {
            user = "anonymous";
        }
        int timeout = Math.max(500, timeoutMillis);
        boolean credentialsSent = false;
        try (Socket socket = new Socket()) {
            socket.connect(new InetSocketAddress(host.trim(), port), timeout);
            socket.setSoTimeout(timeout);
            BufferedReader reader =
                    new BufferedReader(
                            new InputStreamReader(socket.getInputStream(), StandardCharsets.UTF_8));
            OutputStream out = socket.getOutputStream();
            String greeting = readFtpReply(reader);
            if (!greeting.startsWith("220")) {
                return failedMessage(method, "FTP 服务未就绪: " + greeting);
            }
            sendFtp(out, "USER " + user);
            String reply = readFtpReply(reader);
            if (reply.startsWith("331") || reply.startsWith("332")) {
                sendFtp(out, "PASS " + StringUtils.defaultString(password));
                credentialsSent = true;
                reply = readFtpReply(reader);
            } else if (reply.startsWith("230") || reply.startsWith("202")) {
                // USER alone accepted (rare); treat as success below.
            } else if (reply.startsWith("530") || reply.startsWith("430")) {
                return failedMessage(method, "认证失败 (authentication failed): " + reply);
            }
            if (reply.startsWith("230") || reply.startsWith("202")) {
                try {
                    sendFtp(out, "QUIT");
                } catch (IOException ignored) {
                    // no-op
                }
                Map<String, Object> res = new LinkedHashMap<>();
                res.put("status", "success");
                res.put("probeMethod", method);
                res.put("message", "FTP login succeeded (" + host.trim() + ":" + port + ").");
                return res;
            }
            if (reply.startsWith("530") || reply.startsWith("430")) {
                return failedMessage(method, "认证失败 (authentication failed): " + reply);
            }
            return failedMessage(method, "FTP 登录失败: " + reply);
        } catch (java.io.InterruptedIOException e) {
            // vsftpd+PAM 常把错误密码的 530 延迟到接近探测超时；超时/中断发生在 PASS 之后时按认证失败提示，
            // 避免生产默认 3500ms 下被误报为纯 timeout。
            // vsftpd+PAM often delays 530 for bad passwords until near the probe deadline; when the
            // timeout/interrupt happens after PASS, surface it as a possible auth failure rather
            // than a bare timeout.
            if (credentialsSent) {
                return failedMessage(
                        method,
                        "认证失败 (authentication failed): FTP 登录在超时前未返回应答，"
                                + "可能是凭据错误触发的延迟拒绝 (如 vsftpd 530 Login incorrect)。"
                                + "请核对用户名密码，或适当增大超时。");
            }
            return failed(method, e);
        } catch (Throwable t) {
            return failed(method, t);
        }
    }

    private static void sendFtp(OutputStream out, String command) throws IOException {
        out.write((command + "\r\n").getBytes(StandardCharsets.UTF_8));
        out.flush();
    }

    /** 读取一条（可能多行的）FTP 应答。 Reads one (possibly multi-line) FTP reply. */
    static String readFtpReply(BufferedReader reader) throws IOException {
        String line = reader.readLine();
        if (line == null) {
            throw new IOException("FTP server closed the connection without a reply");
        }
        if (line.length() >= 4 && line.charAt(3) == '-') {
            String code = line.substring(0, 3);
            String next;
            int guard = 0;
            while ((next = reader.readLine()) != null && guard++ < 200) {
                if (next.startsWith(code + " ")) {
                    return next.trim();
                }
            }
        }
        return line.trim();
    }

    // ------------------------------------------------------------------- SFTP

    static Map<String, Object> probeSftp(
            Config config, ClassLoader classLoader, int timeoutMillis) {
        String method = "sftp-auth";
        String host = ConnectionProbeSupport.getString(config, "host");
        if (StringUtils.isBlank(host)) {
            return null;
        }
        int port = parsePort(ConnectionProbeSupport.getString(config, "port"), 22);
        String user = firstNonBlank(config, "user", "username");
        String password = ConnectionProbeSupport.getString(config, "password");
        Class<?> jschClass;
        try {
            jschClass = Class.forName("com.jcraft.jsch.JSch", true, classLoader);
        } catch (Throwable e) {
            return unverified(method, "未找到 JSch（SftpFile 连接器未安装？），SFTP 认证信息未验证。");
        }
        int timeout = Math.max(500, timeoutMillis);
        Object session = null;
        Class<?> sessionClass = null;
        try {
            Object jsch = jschClass.getDeclaredConstructor().newInstance();
            session =
                    jschClass
                            .getMethod("getSession", String.class, String.class, int.class)
                            .invoke(jsch, user, host.trim(), port);
            sessionClass =
                    Class.forName("com.jcraft.jsch.Session", false, jschClass.getClassLoader());
            if (password != null) {
                sessionClass.getMethod("setPassword", String.class).invoke(session, password);
            }
            Properties props = new Properties();
            props.put("StrictHostKeyChecking", "no");
            if (password != null) {
                props.put("PreferredAuthentications", "password,keyboard-interactive");
            }
            sessionClass.getMethod("setConfig", Properties.class).invoke(session, props);
            sessionClass.getMethod("setTimeout", int.class).invoke(session, timeout);
            sessionClass.getMethod("connect", int.class).invoke(session, timeout);
            Object channel =
                    sessionClass.getMethod("openChannel", String.class).invoke(session, "sftp");
            Class<?> channelClass =
                    Class.forName("com.jcraft.jsch.Channel", false, jschClass.getClassLoader());
            channelClass.getMethod("connect", int.class).invoke(channel, timeout);
            channelClass.getMethod("disconnect").invoke(channel);
            Map<String, Object> res = new LinkedHashMap<>();
            res.put("status", "success");
            res.put("probeMethod", method);
            res.put("message", "SFTP authentication succeeded (" + host.trim() + ":" + port + ").");
            return res;
        } catch (Throwable t) {
            Throwable cause = unwrap(t);
            String detail = ConnectionProbeSupport.describeFailure(cause);
            String lower = detail == null ? "" : detail.toLowerCase(Locale.ROOT);
            // OpenSSH PerSourcePenalties 在连续认证失败后会对来源施加约 15s 惩罚并重置连接。
            // OpenSSH PerSourcePenalties can reset the connection after repeated auth failures.
            if (lower.contains("connection reset")
                    || lower.contains("closed by foreign host")
                    || lower.contains("connection is closed")) {
                detail =
                        detail
                                + " (possible SSH source penalty after failed authentication;"
                                + " retry later or check credentials; disable PerSourcePenalties in tests)";
            }
            return failedMessage(method, detail);
        } finally {
            if (session != null && sessionClass != null) {
                try {
                    sessionClass.getMethod("disconnect").invoke(session);
                } catch (Throwable ignored) {
                    // no-op
                }
            }
        }
    }

    // ---------------------------------------------------------------- helpers

    private static Throwable unwrap(Throwable t) {
        Throwable current = t;
        for (int i = 0; i < 5; i++) {
            if (current instanceof InvocationTargetException
                    && ((InvocationTargetException) current).getTargetException() != null) {
                current = ((InvocationTargetException) current).getTargetException();
            } else if (current instanceof ExecutionException && current.getCause() != null) {
                current = current.getCause();
            } else {
                break;
            }
        }
        return current;
    }

    private static Map<String, Object> failed(String method, Throwable t) {
        return failedMessage(method, ConnectionProbeSupport.describeFailure(t));
    }

    private static Map<String, Object> failedMessage(String method, String message) {
        Map<String, Object> res = new LinkedHashMap<>();
        res.put("status", "failed");
        res.put("probeMethod", method);
        res.put("message", message);
        return res;
    }

    /** 无法验证认证信息：不报 success，以 warning 级 skipped 呈现。 Credentials could not be verified. */
    private static Map<String, Object> unverified(String method, String message) {
        Map<String, Object> res = new LinkedHashMap<>();
        res.put("status", "skipped");
        res.put("severity", "warning");
        res.put("probeMethod", method);
        res.put("message", message);
        return res;
    }

    private static int parsePort(String value, int defaultPort) {
        if (StringUtils.isBlank(value)) {
            return defaultPort;
        }
        try {
            return Integer.parseInt(value.trim());
        } catch (NumberFormatException e) {
            return defaultPort;
        }
    }

    private static String firstNonBlank(Config config, String... keys) {
        for (String key : keys) {
            String v = ConnectionProbeSupport.getString(config, key);
            if (StringUtils.isNotBlank(v)) {
                return v;
            }
        }
        return null;
    }
}
