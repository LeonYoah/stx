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
 * Unit tests for connector-quick-helper
 * 连接器快速辅助工具单元测试
 */

import {describe, expect, it} from 'vitest';
import {
  generateConnectorDocUrls,
  getConnectorCapability,
  getConnectorHoconIdentifier,
  getConnectorProcessingModes,
  getConnectorSemanticIcon,
  matchesConnectorProcessingMode,
} from '../connector-quick-helper';

describe('connector-quick-helper', () => {
  describe('getConnectorHoconIdentifier', () => {
    it('resolves CDC connectors to standard uppercase format', () => {
      expect(getConnectorHoconIdentifier('cdc-mysql')).toBe('MySQL-CDC');
      expect(getConnectorHoconIdentifier('cdc-oracle')).toBe('Oracle-CDC');
      expect(getConnectorHoconIdentifier('cdc-postgres')).toBe('Postgres-CDC');
      expect(getConnectorHoconIdentifier('cdc-sqlserver')).toBe('SqlServer-CDC');
    });

    it('resolves special-cased connectors correctly', () => {
      expect(getConnectorHoconIdentifier('jdbc')).toBe('Jdbc');
      expect(getConnectorHoconIdentifier('kafka')).toBe('Kafka');
      expect(getConnectorHoconIdentifier('clickhouse')).toBe('Clickhouse');
      expect(getConnectorHoconIdentifier('doris')).toBe('Doris');
      expect(getConnectorHoconIdentifier('starrocks')).toBe('StarRocks');
      expect(getConnectorHoconIdentifier('elasticsearch')).toBe('Elasticsearch');
      expect(getConnectorHoconIdentifier('file-s3')).toBe('S3File');
    });

    it('converts generic names to PascalCase as fallback', () => {
      expect(getConnectorHoconIdentifier('custom-source')).toBe('CustomSource');
    });
  });

  describe('getConnectorCapability', () => {
    it('marks CDC connectors as source-only', () => {
      const cap = getConnectorCapability('cdc-mysql');
      expect(cap.supportsSource).toBe(true);
      expect(cap.supportsSink).toBe(false);
      expect(cap.isTransform).toBe(false);
    });

    it('marks console and assert as sink-only', () => {
      const cap = getConnectorCapability('console');
      expect(cap.supportsSource).toBe(false);
      expect(cap.supportsSink).toBe(true);
    });

    it('marks fake as source-only', () => {
      const cap = getConnectorCapability('fake');
      expect(cap.supportsSource).toBe(true);
      expect(cap.supportsSink).toBe(false);
    });

    it('marks transform as transform', () => {
      const cap = getConnectorCapability('seatunnel-transforms-v2', 'transform');
      expect(cap.isTransform).toBe(true);
      expect(cap.supportsSource).toBe(false);
      expect(cap.supportsSink).toBe(false);
    });

    it('marks common connectors as supporting both source and sink', () => {
      const cap = getConnectorCapability('kafka');
      expect(cap.supportsSource).toBe(true);
      expect(cap.supportsSink).toBe(true);
    });
  });



  describe('generateConnectorDocUrls', () => {
    it('builds source doc url for CDC connector with Chinese locale', () => {
      const urls = generateConnectorDocUrls('cdc-mysql', '2.3.13', 'zh');
      expect(urls.sourceDocUrl).toBe(
        'https://seatunnel.apache.org/zh-CN/docs/connectors/source/MySQL-CDC/',
      );
      expect(urls.sinkDocUrl).toBeUndefined();
    });

    it('builds both source and sink urls for dual-capability connector in English', () => {
      const urls = generateConnectorDocUrls('jdbc', '2.3.12', 'en');
      expect(urls.sourceDocUrl).toBe(
        'https://seatunnel.apache.org/docs/connectors/source/Jdbc/',
      );
      expect(urls.sinkDocUrl).toBe(
        'https://seatunnel.apache.org/docs/connectors/sink/Jdbc/',
      );
    });

    it('builds transform doc url correctly with locale synchronization', () => {
      const zhUrls = generateConnectorDocUrls('seatunnel-transforms-v2', '2.3.13', 'zh', 'transform');
      expect(zhUrls.sourceDocUrl).toBe(
        'https://seatunnel.apache.org/zh-CN/docs/transforms/sql/',
      );

      const enUrls = generateConnectorDocUrls('seatunnel-transforms-v2', '2.3.13', 'en', 'transform');
      expect(enUrls.sourceDocUrl).toBe(
        'https://seatunnel.apache.org/docs/transforms/sql/',
      );
    });
  });

  describe('processing mode classification', () => {
    it('correctly classifies CDC as realtime streaming only', () => {
      expect(getConnectorProcessingModes('cdc-mysql')).toEqual(['realtime']);
      expect(matchesConnectorProcessingMode('cdc-mysql', 'realtime')).toBe(true);
      expect(matchesConnectorProcessingMode('cdc-mysql', 'offline')).toBe(false);
      expect(matchesConnectorProcessingMode('cdc-mysql', 'all')).toBe(true);
    });

    it('classifies message queues (Kafka, Pulsar) and Lakehouses (Iceberg) as dual-mode', () => {
      expect(getConnectorProcessingModes('kafka')).toEqual(['realtime', 'offline']);
      expect(matchesConnectorProcessingMode('kafka', 'realtime')).toBe(true);
      expect(matchesConnectorProcessingMode('kafka', 'offline')).toBe(true);

      expect(getConnectorProcessingModes('iceberg')).toEqual(['realtime', 'offline']);
      expect(matchesConnectorProcessingMode('iceberg', 'realtime')).toBe(true);
      expect(matchesConnectorProcessingMode('iceberg', 'offline')).toBe(true);
    });

    it('classifies JDBC and files as offline batch', () => {
      expect(getConnectorProcessingModes('jdbc')).toEqual(['offline']);
      expect(matchesConnectorProcessingMode('jdbc', 'realtime')).toBe(false);
      expect(matchesConnectorProcessingMode('jdbc', 'offline')).toBe(true);

      expect(getConnectorProcessingModes('file-s3')).toEqual(['offline']);
      expect(matchesConnectorProcessingMode('file-s3', 'realtime')).toBe(false);
      expect(matchesConnectorProcessingMode('file-s3', 'offline')).toBe(true);
    });

    it('supports legacy connector category filter gracefully', () => {
      expect(matchesConnectorProcessingMode('jdbc', 'connector')).toBe(true);
      expect(matchesConnectorProcessingMode('cdc-mysql', 'connector')).toBe(true);
    });
  });

  describe('getConnectorSemanticIcon', () => {
    it('returns appropriate icons, badges, and official icon URLs for different categories', () => {
      const kafka = getConnectorSemanticIcon('kafka');
      expect(kafka.tagLabel).toBe('Messaging');
      expect(kafka.iconUrl).toBe('/icons/connectors/Kafka.svg');

      const clickhouse = getConnectorSemanticIcon('clickhouse');
      expect(clickhouse.tagLabel).toBe('Warehouse');
      expect(clickhouse.iconUrl).toBe('/icons/connectors/Clickhouse.png');

      const cdc = getConnectorSemanticIcon('cdc-mysql');
      expect(cdc.tagLabel).toBe('CDC Stream');
      expect(cdc.iconUrl).toBe('/icons/connectors/mysql-cdc.svg');

      const jdbc = getConnectorSemanticIcon('jdbc');
      expect(jdbc.tagLabel).toBe('RDBMS');
      expect(jdbc.iconUrl).toBe('/icons/connectors/JDBC.svg');

      const s3 = getConnectorSemanticIcon('file-s3');
      expect(s3.tagLabel).toBe('Storage');
      expect(s3.iconUrl).toBe('/icons/connectors/S3File.svg');

      const transform = getConnectorSemanticIcon('transform', 'transform');
      expect(transform.tagLabel).toBe('Transform');
    });
  });
});
