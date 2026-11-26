import {describe, expect, it} from 'vitest';
import {resolveImageSeatunnelVersion} from '../image.service';

describe('resolveImageSeatunnelVersion', () => {
  it('registered image returns its recorded version', () => {
    expect(
      resolveImageSeatunnelVersion('corp/seatunnel:custom', [
        {image: 'corp/seatunnel:custom', seatunnel_version: '2.3.13'},
      ]),
    ).toBe('2.3.13');
  });
  it('extracts x.y.z tag', () => {
    expect(resolveImageSeatunnelVersion('apache/seatunnel:2.3.13')).toBe('2.3.13');
    expect(resolveImageSeatunnelVersion('apache/seatunnel:2.3.13-rc1')).toBe('2.3.13-rc1');
  });
  it('embedded version for seatunnel images without clean tag', () => {
    expect(resolveImageSeatunnelVersion('corp/seatunnel-2.3.13@sha256:abc')).toBe('2.3.13');
  });
  it('unrecognizable returns empty', () => {
    expect(resolveImageSeatunnelVersion('corp/seatunnel:latest')).toBe('');
    expect(resolveImageSeatunnelVersion('')).toBe('');
  });
});
