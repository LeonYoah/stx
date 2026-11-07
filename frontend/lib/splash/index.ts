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

export {
  clearPostLoginSplashSession,
  consumePostLoginSplashPending,
  markPostLoginSplashPending,
  peekPostLoginSplashPending,
  POST_LOGIN_SPLASH_PENDING_KEY,
} from './post-login-splash-session';

export {
  MeteorIntroEngine,
  REDUCED_MOTION_HOLD_MS,
  SPLASH_BG_SRC,
  SPLASH_MARK_SRC,
  SPLASH_TIMELINE,
  type MeteorIntroEngineOptions,
} from './meteor-intro-engine';
