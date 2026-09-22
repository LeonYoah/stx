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

package io.github.leonyoah.stx.proxy.adapter;

import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import java.io.Serializable;
import java.util.Objects;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Parses and represents SeaTunnel version information (e.g. 2.3.13, 3.0.0, 3.1.0) and supports
 * wildcard/prefix pattern matching for version routing.
 */
public final class EngineVersion implements Comparable<EngineVersion>, Serializable {

    private static final long serialVersionUID = 1L;
    private static final Pattern VERSION_PATTERN =
            Pattern.compile("^v?(\\d+)(?:\\.(\\d+))?(?:\\.(\\d+))?.*$");

    private final String raw;
    private final int major;
    private final int minor;
    private final int patch;

    private EngineVersion(String raw, int major, int minor, int patch) {
        this.raw = raw != null ? raw.trim() : "";
        this.major = major;
        this.minor = minor;
        this.patch = patch;
    }

    public static EngineVersion parse(String versionStr) {
        if (StringUtils.isBlank(versionStr)) {
            return new EngineVersion("unknown", 0, 0, 0);
        }
        String trimmed = versionStr.trim();
        String normalized =
                (trimmed.startsWith("v") || trimmed.startsWith("V"))
                        ? trimmed.substring(1)
                        : trimmed;
        Matcher matcher = VERSION_PATTERN.matcher(trimmed);
        if (matcher.matches()) {
            int major = Integer.parseInt(matcher.group(1));
            int minor = matcher.group(2) != null ? Integer.parseInt(matcher.group(2)) : 0;
            int patch = matcher.group(3) != null ? Integer.parseInt(matcher.group(3)) : 0;
            return new EngineVersion(normalized, major, minor, patch);
        }
        return new EngineVersion(normalized, 0, 0, 0);
    }

    public static EngineVersion of(int major, int minor, int patch) {
        return new EngineVersion(major + "." + minor + "." + patch, major, minor, patch);
    }

    public String getRaw() {
        return raw;
    }

    public int getMajor() {
        return major;
    }

    public int getMinor() {
        return minor;
    }

    public int getPatch() {
        return patch;
    }

    public boolean isVersion3x() {
        return major == 3;
    }

    public boolean isVersion2x() {
        return major == 2;
    }

    /** Matches this version against a rule pattern (e.g. "3.0.*", "3.*", "2.3.13", "*"). */
    public boolean matches(String pattern) {
        if (StringUtils.isBlank(pattern) || "*".equals(pattern.trim())) {
            return true;
        }
        String p = pattern.trim();
        if (p.startsWith("v") || p.startsWith("V")) {
            p = p.substring(1);
        }
        if (p.endsWith(".*")) {
            String prefix = p.substring(0, p.length() - 2);
            return raw.equals(prefix) || raw.startsWith(prefix + ".");
        }
        if (p.endsWith("*")) {
            String prefix = p.substring(0, p.length() - 1);
            return raw.startsWith(prefix);
        }
        return raw.equalsIgnoreCase(p);
    }

    @Override
    public int compareTo(EngineVersion other) {
        if (other == null) {
            return 1;
        }
        if (this.major != other.major) {
            return Integer.compare(this.major, other.major);
        }
        if (this.minor != other.minor) {
            return Integer.compare(this.minor, other.minor);
        }
        return Integer.compare(this.patch, other.patch);
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        EngineVersion that = (EngineVersion) o;
        return major == that.major && minor == that.minor && patch == that.patch;
    }

    @Override
    public int hashCode() {
        return Objects.hash(major, minor, patch);
    }

    @Override
    public String toString() {
        return raw;
    }
}
