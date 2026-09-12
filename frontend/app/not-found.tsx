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

'use client';
import {motion} from 'motion/react';
import {Button} from '@/components/ui/button';
import {MessageCircle, Home, Ship} from 'lucide-react';
import Link from 'next/link';

export default function NotFound() {
  return (
    <div className='fixed inset-0 flex items-center justify-center dark:bg-black bg-white'>
      <div className='max-w-7xl mx-auto text-center px-4'>
        {/* 404 大数字 */}
        <motion.div
          initial={{opacity: 0, scale: 0.8}}
          animate={{opacity: 1, scale: 1}}
          transition={{duration: 0.5}}
          className='mb-6'
        >
          <span className='text-8xl md:text-9xl font-bold bg-gradient-to-r from-blue-500 to-cyan-500 bg-clip-text text-transparent'>
            404
          </span>
        </motion.div>

        {/* 标题动画 */}
        <motion.div
          initial={{opacity: 0, y: 20}}
          animate={{opacity: 1, y: 0}}
          transition={{duration: 0.6, delay: 0.2}}
        >
          <p className='font-bold text-xl md:text-4xl dark:text-white text-black flex items-center justify-center gap-2'>
            <Ship className='w-6 h-6 md:w-8 md:h-8 text-blue-500' />
            {'页面'.split('').map((word, idx) => (
              <motion.span
                key={idx}
                className='inline-block'
                initial={{x: -10, opacity: 0}}
                animate={{x: 0, opacity: 1}}
                transition={{duration: 0.5, delay: idx * 0.04}}
              >
                {word}
              </motion.span>
            ))}
            <span className='text-neutral-400'>
              {'迷航了'.split('').map((word, idx) => (
                <motion.span
                  key={idx}
                  className='inline-block'
                  initial={{x: -10, opacity: 0}}
                  animate={{x: 0, opacity: 1}}
                  transition={{duration: 0.5, delay: (idx + 2) * 0.04}}
                >
                  {word}
                </motion.span>
              ))}
            </span>
          </p>
        </motion.div>

        {/* 描述文字 */}
        <motion.div
          initial={{opacity: 0, y: 20}}
          animate={{opacity: 1, y: 0}}
          transition={{duration: 0.6, delay: 0.6}}
        >
          <p className='text-sm md:text-lg text-neutral-500 max-w-2xl mx-auto py-4'>
            {'抱歉，您访问的页面不存在或已被移动。'
              .split('')
              .map((char, idx) => (
                <motion.span
                  key={idx}
                  className='inline-block'
                  initial={{opacity: 0}}
                  animate={{opacity: 1}}
                  transition={{duration: 0.02, delay: 0.8 + idx * 0.01}}
                >
                  {char === ' ' ? '\u00A0' : char}
                </motion.span>
              ))}
            <br />
            <motion.span
              initial={{opacity: 0}}
              animate={{opacity: 1}}
              transition={{duration: 0.3, delay: 1.2}}
              className='inline-block mt-2'
            >
              {'如有问题，请在 GitHub 上'.split('').map((char, idx) => (
                <motion.span
                  key={idx}
                  className='inline-block'
                  initial={{opacity: 0}}
                  animate={{opacity: 1}}
                  transition={{duration: 0.02, delay: 1.3 + idx * 0.01}}
                >
                  {char === ' ' ? '\u00A0' : char}
                </motion.span>
              ))}
            </motion.span>
            <motion.span
              initial={{opacity: 0}}
              animate={{opacity: 1}}
              transition={{duration: 0.3, delay: 1.6}}
              className='inline-block'
            >
              <a
                href='https://github.com/LeonYoah/stx/issues/new/choose'
                target='_blank'
                rel='noopener noreferrer'
                className='text-blue-500 hover:text-blue-600 underline mx-1 inline-flex items-center gap-1'
              >
                <MessageCircle className='w-3 h-3' />
                提交反馈
              </a>
            </motion.span>
          </p>
        </motion.div>

        {/* 返回首页按钮 */}
        <motion.div
          initial={{opacity: 0, y: 20}}
          animate={{opacity: 1, y: 0}}
          transition={{duration: 0.6, delay: 2.0}}
          className='flex justify-center pt-6'
        >
          <Link href='/dashboard'>
            <Button
              size='lg'
              className='bg-gradient-to-r from-blue-500 to-cyan-500 hover:from-blue-600 hover:to-cyan-600'
            >
              <Home className='w-4 h-4 mr-2' />
              返回控制台
            </Button>
          </Link>
        </motion.div>

        {/* 底部装饰 */}
        <motion.div
          initial={{opacity: 0}}
          animate={{opacity: 0.5}}
          transition={{duration: 1, delay: 2.5}}
          className='mt-12 text-xs text-neutral-400'
        >
          STX - Apache SeaTunnel 一站式运维管理平台
        </motion.div>
      </div>
    </div>
  );
}
