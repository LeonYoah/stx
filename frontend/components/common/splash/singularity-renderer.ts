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
 * 开屏奇点漩涡背景的 WebGL2 渲染器（STX 原创着色器）。
 * WebGL2 renderer for the splash singularity vortex (original STX shader).
 *
 * 历史上曾考虑移植 Inspira UI / XorDev ShaderToy「Singularity」(3csSWB)。
 * 经核实：ShaderToy 未在源码中声明许可时默认 CC BY-NC-SA 3.0；Inspira 的 MIT
 * 仅覆盖其 Vue 封装，不能覆盖该着色器原作。CC BY-NC-SA 与本仓库 Apache-2.0
 * 商业分发不兼容，故不保留该算法，也不伪造 ASF/Apache 归属。
 * We previously considered porting Inspira UI / XorDev ShaderToy "Singularity"
 * (3csSWB). ShaderToy's default when no license is pasted is CC BY-NC-SA 3.0;
 * Inspira's MIT covers only its Vue wrapper, not that shader. CC BY-NC-SA is
 * incompatible with this repo's Apache-2.0 distribution, so that algorithm is
 * not kept and we do not forge ASF/Apache authorship over it.
 *
 * 下方 GLSL 为 STX 自行编写的极坐标螺线吸积盘，仅保留开屏所需 uniform。
 * The GLSL below is an original STX polar-spiral accretion disk; only splash
 * uniforms are retained.
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
uniform float uZoom;

out vec4 fragColor;

// STX 原创极坐标螺线吸积盘（非 XorDev/Inspira 算法移植）。
// Original STX polar-spiral accretion disk (not a XorDev/Inspira port).
vec3 stxVortex(vec2 F, vec2 r, float t, out vec2 pOut) {
  vec2 p = ((F - 0.5 * r) / max(r.y, 1.) - uOffset) * uZoom;
  pOut = p;
  float rad = length(p);
  float ang = atan(p.y, p.x);
  float arms = 0.;
  for (int k = 1; k <= 5; k++) {
    float fk = float(k);
    arms += 0.5 + 0.5 * sin(
      ang * (1.35 + fk * 0.55)
      - log(rad + 0.075) * (2.4 + fk * 0.65)
      + t * (0.42 + fk * 0.11)
    );
  }
  arms *= 0.2;
  float disk = smoothstep(0.92, 0.28, rad) * smoothstep(0.015, 0.16, rad);
  float rim = exp(-pow((rad - 0.52) * 9.5, 2.));
  float core = exp(-rad * rad * 22.);
  float haze = exp(-rad * 1.8) * 0.12;
  float l = disk * (0.16 + 0.62 * arms) + rim * 1.15 + core * 0.18 + haze;
  l = pow(clamp(l, 0., 1.4), 0.92);
  return l * vec3(0.97, 0.985, 1.0);
}

void main() {
  vec2 p;
  vec3 col = stxVortex(gl_FragCoord.xy, uResolution, uTime, p);
  col *= uIntensity * (1. + uFlash * .8);
  // 流星穿心时的中心白热闪光 / White-hot core flash as the meteor passes through.
  col += uFlash * exp(-dot(p, p) * 4.);
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
  /** 内爆缩放：>1 时漩涡向中心收缩 / Implosion zoom: >1 pulls the vortex inward. */
  zoom: number;
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
    zoom: WebGLUniformLocation | null;
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
      zoom: gl.getUniformLocation(program, 'uZoom'),
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
    gl.uniform1f(this.uniforms.zoom, frame.zoom);
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
