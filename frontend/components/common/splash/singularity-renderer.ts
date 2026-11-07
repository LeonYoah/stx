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
 * 奇点漩涡背景的 WebGL2 渲染器。
 * WebGL2 renderer for the singularity vortex background.
 *
 * 着色器移植自 Inspira UI 的 Singularity Background（MIT），
 * 原作为 XorDev 的 ShaderToy “Singularity”（https://www.shadertoy.com/view/3csSWB）。
 * 在原算法外只加了开屏需要的统一变量：鼠标视差偏移、整体亮度、穿心闪光。
 * Shader ported from Inspira UI's Singularity Background (MIT), originally
 * "Singularity" by XorDev on ShaderToy. Only splash uniforms were added on top
 * of the original math: pointer parallax offset, global intensity, impact flash.
 *
 * 直接使用原生 WebGL2 画一个全屏三角形，不额外引入 ogl 等依赖。
 * Draws one full-screen triangle with raw WebGL2; no extra dependency like ogl.
 */

const VERTEX_SHADER = `#version 300 es
in vec2 aPosition;
void main() {
  gl_Position = vec4(aPosition, 0.0, 1.0);
}
`;

const FRAGMENT_SHADER = `#version 300 es
precision highp float;

uniform vec2 uResolution;
uniform float uTime;
uniform vec2 uOffset;
uniform float uIntensity;
uniform float uFlash;

out vec4 fragColor;

// "Singularity" by @XorDev — 原始算法保持不变 / original math kept intact.
vec3 singularity(vec2 F, vec2 r, float t, out vec2 pOut) {
  float i = .2, a;
  vec2 p = (F + F - r) / r.y / .7 - uOffset,
    d = vec2(-1, 1),
    b = p - i * d,
    c = p * mat2(1, 1, d / (.1 + i / dot(b, b))),
    v = c * mat2(cos(.5 * log(a = dot(c, c)) + t * i + vec4(0, 33, 11, 0))) / i,
    w = vec2(0);
  for (; i++ < 9.; w += 1. + sin(v)) {
    v += .7 * sin(v.yx * i + t) / i + .5;
  }
  i = length(sin(v / .3) * .4 + c * (3. + d));
  vec4 O = 1. - exp(-exp(c.x * vec4(.6, -.4, -1, 0))
    / w.xyyx
    / (2. + i * i / 4. - i)
    / (.5 + 1. / a)
    / (.03 + abs(length(p) - .7)));
  pOut = p;
  return O.rgb;
}

void main() {
  vec2 p;
  vec3 col = singularity(gl_FragCoord.xy, uResolution, uTime, p);
  col *= uIntensity * (1. + uFlash * .6);
  // 流星穿心时的中心白热闪光 / White-hot core flash as the meteor passes through.
  col += uFlash * vec3(1., .94, .9) * exp(-dot(p, p) * 5.);
  fragColor = vec4(col, 1.);
}
`;

/** 每帧渲染参数 / Per-frame render parameters. */
export interface SingularityFrame {
  /** 着色器时间（秒）/ Shader time in seconds. */
  time: number;
  /** 视差偏移（着色器 p 空间）/ Parallax offset in shader p-space. */
  offset: [number, number];
  intensity: number;
  flash: number;
}

function compileShader(
  gl: WebGL2RenderingContext,
  type: number,
  source: string,
): WebGLShader {
  const shader = gl.createShader(type);
  if (!shader) {
    throw new Error('createShader failed');
  }
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    const log = gl.getShaderInfoLog(shader);
    gl.deleteShader(shader);
    throw new Error(`shader compile failed: ${log ?? ''}`);
  }
  return shader;
}

export class SingularityRenderer {
  private readonly gl: WebGL2RenderingContext;
  private readonly program: WebGLProgram;
  private readonly buffer: WebGLBuffer;
  private readonly vao: WebGLVertexArrayObject;
  private readonly uniforms: {
    resolution: WebGLUniformLocation | null;
    time: WebGLUniformLocation | null;
    offset: WebGLUniformLocation | null;
    intensity: WebGLUniformLocation | null;
    flash: WebGLUniformLocation | null;
  };

  /**
   * 创建渲染器；WebGL2 不可用或编译失败时抛错，由调用方降级为 CSS 漩涡。
   * Create the renderer; throws when WebGL2 is unavailable or compilation
   * fails so the caller can fall back to the CSS vortex.
   */
  constructor(readonly canvas: HTMLCanvasElement) {
    const gl = canvas.getContext('webgl2', {
      alpha: false,
      antialias: false,
      depth: false,
      stencil: false,
      premultipliedAlpha: false,
      // 碎裂时需要同帧截图，保持默认 false 即可（截图在同一回调内完成）。
      // Shatter snapshots happen in the same callback, so false is enough.
      preserveDrawingBuffer: false,
      powerPreference: 'high-performance',
    });
    if (!gl) {
      throw new Error('WebGL2 not supported');
    }
    this.gl = gl;

    const vs = compileShader(gl, gl.VERTEX_SHADER, VERTEX_SHADER);
    const fs = compileShader(gl, gl.FRAGMENT_SHADER, FRAGMENT_SHADER);
    const program = gl.createProgram();
    if (!program) {
      throw new Error('createProgram failed');
    }
    gl.attachShader(program, vs);
    gl.attachShader(program, fs);
    gl.linkProgram(program);
    gl.deleteShader(vs);
    gl.deleteShader(fs);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      const log = gl.getProgramInfoLog(program);
      gl.deleteProgram(program);
      throw new Error(`program link failed: ${log ?? ''}`);
    }
    this.program = program;

    // 覆盖整个视口的单个大三角形 / One oversized triangle covering the viewport.
    const buffer = gl.createBuffer();
    const vao = gl.createVertexArray();
    if (!buffer || !vao) {
      throw new Error('buffer allocation failed');
    }
    this.buffer = buffer;
    this.vao = vao;
    gl.bindVertexArray(vao);
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
    gl.bufferData(
      gl.ARRAY_BUFFER,
      new Float32Array([-1, -1, 3, -1, -1, 3]),
      gl.STATIC_DRAW,
    );
    const loc = gl.getAttribLocation(program, 'aPosition');
    gl.enableVertexAttribArray(loc);
    gl.vertexAttribPointer(loc, 2, gl.FLOAT, false, 0, 0);
    gl.bindVertexArray(null);

    this.uniforms = {
      resolution: gl.getUniformLocation(program, 'uResolution'),
      time: gl.getUniformLocation(program, 'uTime'),
      offset: gl.getUniformLocation(program, 'uOffset'),
      intensity: gl.getUniformLocation(program, 'uIntensity'),
      flash: gl.getUniformLocation(program, 'uFlash'),
    };
  }

  /**
   * 按 CSS 尺寸与像素比调整画布。着色器较重，像素比上限为 1，大屏再降采样。
   * Resize by CSS size and pixel ratio. The shader is heavy, so the ratio is
   * capped at 1 and further reduced on very large screens.
   */
  resize(cssWidth: number, cssHeight: number): void {
    const area = cssWidth * cssHeight;
    const ratio =
      Math.min(window.devicePixelRatio || 1, 1) * (area > 2_400_000 ? 0.75 : 1);
    const w = Math.max(1, Math.round(cssWidth * ratio));
    const h = Math.max(1, Math.round(cssHeight * ratio));
    if (this.canvas.width !== w || this.canvas.height !== h) {
      this.canvas.width = w;
      this.canvas.height = h;
    }
    this.gl.viewport(0, 0, w, h);
  }

  render(frame: SingularityFrame): void {
    const {gl} = this;
    gl.useProgram(this.program);
    gl.bindVertexArray(this.vao);
    gl.uniform2f(
      this.uniforms.resolution,
      this.canvas.width,
      this.canvas.height,
    );
    gl.uniform1f(this.uniforms.time, frame.time);
    gl.uniform2f(this.uniforms.offset, frame.offset[0], frame.offset[1]);
    gl.uniform1f(this.uniforms.intensity, frame.intensity);
    gl.uniform1f(this.uniforms.flash, frame.flash);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
    gl.bindVertexArray(null);
  }

  /**
   * 释放 GPU 资源。不主动 loseContext：开发态 StrictMode 会在同一画布上重建渲染器，
   * 丢弃上下文会让第二次 getContext 拿到失效上下文；画布卸载后由浏览器回收。
   * Free GPU resources. We intentionally skip loseContext: StrictMode in dev
   * rebuilds the renderer on the same canvas, and a lost context would break
   * that; the browser reclaims it once the canvas unmounts.
   */
  dispose(): void {
    const {gl} = this;
    gl.deleteBuffer(this.buffer);
    gl.deleteVertexArray(this.vao);
    gl.deleteProgram(this.program);
  }
}
