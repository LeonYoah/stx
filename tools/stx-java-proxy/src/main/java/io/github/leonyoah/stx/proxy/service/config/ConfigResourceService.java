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

import io.github.leonyoah.stx.proxy.model.DagParseResult;
import io.github.leonyoah.stx.proxy.model.JobConfigContext;

import java.util.Map;

public class ConfigResourceService {

    private final JobConfigSupportService jobConfigSupportService;
    private final WebUiDagPreviewService webUiDagPreviewService;

    public ConfigResourceService() {
        this(new JobConfigSupportService(), null);
    }

    ConfigResourceService(JobConfigSupportService jobConfigSupportService) {
        this(jobConfigSupportService, null);
    }

    ConfigResourceService(
            JobConfigSupportService jobConfigSupportService,
            WebUiDagPreviewService webUiDagPreviewService) {
        this.jobConfigSupportService = jobConfigSupportService;
        this.webUiDagPreviewService =
                webUiDagPreviewService == null
                        ? new WebUiDagPreviewService(jobConfigSupportService)
                        : webUiDagPreviewService;
    }

    public DagParseResult inspectDag(Map<String, Object> request) {
        JobConfigContext context = jobConfigSupportService.parseJobContext(request);
        return new DagParseResult(
                true,
                context.isSimpleGraph(),
                context.getSources().size(),
                context.getTransforms().size(),
                context.getSinks().size(),
                context.getWarnings(),
                context.getGraph());
    }

    public Object inspectWebUiDag(Map<String, Object> request) {
        return webUiDagPreviewService.preview(request);
    }
}
