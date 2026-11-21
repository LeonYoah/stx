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

import {useEffect, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Alert as ArcAlert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {Switch} from '@/components/arc/switch/switch';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import type {RuntimeStorageSpec} from '@/lib/services/cluster/types';

interface RuntimeStorageEditorProps {
  clusterId: number;
  kind: 'checkpoint' | 'imap';
  spec?: RuntimeStorageSpec;
  onApplied?: () => void;
}

const STORAGE_TYPE_OPTIONS = [
  {value: 'LOCAL_FILE', label: 'LOCAL_FILE'},
  {value: 'HDFS', label: 'HDFS'},
  {value: 'OSS', label: 'OSS'},
  {value: 'S3', label: 'S3'},
];
const S3_SIMPLE = 'org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider';
const S3_INSTANCE = 'org.apache.hadoop.fs.s3a.InstanceProfileCredentialsProvider';

/**
 * 按 SeaTunnel plugin-config 区分 HDFS / OSS / S3 字段，全量采用 Arc 表单控件。
 * Visual editor with type-specific SeaTunnel plugin-config fields using Arc UI form primitives.
 */
export function RuntimeStorageEditor({
  clusterId,
  kind,
  spec,
  onApplied,
}: RuntimeStorageEditorProps) {
  const t = useTranslations();
  const [enabled, setEnabled] = useState(true);
  const [storageType, setStorageType] = useState('LOCAL_FILE');
  const [namespace, setNamespace] = useState('');
  const [endpoint, setEndpoint] = useState('');
  const [bucket, setBucket] = useState('');
  const [accessKey, setAccessKey] = useState('');
  const [secretKey, setSecretKey] = useState('');
  const [haEnabled, setHaEnabled] = useState(false);
  const [nameNodeHost, setNameNodeHost] = useState('');
  const [nameNodePort, setNameNodePort] = useState('8020');
  const [nameServices, setNameServices] = useState('');
  const [haNamenodes, setHaNamenodes] = useState('nn1,nn2');
  const [rpc1, setRpc1] = useState('');
  const [rpc2, setRpc2] = useState('');
  const [kerberos, setKerberos] = useState(false);
  const [principal, setPrincipal] = useState('');
  const [keytab, setKeytab] = useState('');
  const [hdfsSitePath, setHdfsSitePath] = useState('');
  const [s3Provider, setS3Provider] = useState(S3_SIMPLE);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  // 保存后 onApplied 会刷新 spec；保留当次保存成功提示不被 spec 刷新误清。
  // onApplied refreshes spec after save; preserve the save notice instead of wiping it immediately.
  const preserveNoticeRef = useRef(false);

  useEffect(() => {
    const type = (
      spec?.storage_type ||
      (kind === 'imap' && spec && !spec.enabled ? 'DISABLED' : 'LOCAL_FILE')
    ).toUpperCase();
    setEnabled(
      kind === 'checkpoint' ? true : Boolean(spec?.enabled ?? type !== 'DISABLED'),
    );
    setStorageType(type === 'DISABLED' ? 'LOCAL_FILE' : type);
    setNamespace(spec?.namespace || '');
    setEndpoint(spec?.endpoint || '');
    setBucket(spec?.bucket || '');
    setAccessKey('');
    setSecretKey('');
    setHaEnabled(Boolean(spec?.hdfs_ha_enabled));
    setNameNodeHost(spec?.hdfs_namenode_host || '');
    setNameNodePort(
      spec?.hdfs_namenode_port ? String(spec.hdfs_namenode_port) : '8020',
    );
    setNameServices(spec?.hdfs_name_services || '');
    setHaNamenodes(spec?.hdfs_ha_namenodes || 'nn1,nn2');
    setRpc1(spec?.hdfs_namenode_rpc_address_1 || '');
    setRpc2(spec?.hdfs_namenode_rpc_address_2 || '');
    setKerberos(
      Boolean(spec?.kerberos_principal || spec?.kerberos_keytab_file_path),
    );
    setPrincipal(spec?.kerberos_principal || '');
    setKeytab(spec?.kerberos_keytab_file_path || '');
    setHdfsSitePath(spec?.hdfs_site_path || '');
    setS3Provider(spec?.s3_credentials_provider || S3_SIMPLE);
    if (preserveNoticeRef.current) {
      preserveNoticeRef.current = false;
    } else {
      setNotice('');
    }
    setError('');
  }, [kind, spec]);

  // 切换存储种类（checkpoint ↔ imap）时清空提示 / Clear feedback when switching kind
  useEffect(() => {
    preserveNoticeRef.current = false;
    setNotice('');
    setError('');
  }, [kind]);

  const handleSave = async () => {
    setSaving(true);
    setError('');
    setNotice('');
    const result = await services.cluster.applyRuntimeStorageSafe(
      clusterId,
      kind,
      {
        enabled: kind === 'imap' ? enabled : true,
        storage_type: kind === 'imap' && !enabled ? 'DISABLED' : storageType,
        namespace,
        endpoint:
          storageType === 'OSS' || storageType === 'S3' ? endpoint : undefined,
        bucket:
          storageType === 'OSS' || storageType === 'S3' ? bucket : undefined,
        access_key: accessKey,
        secret_key: secretKey,
        hdfs_namenode_host:
          storageType === 'HDFS' && !haEnabled ? nameNodeHost : undefined,
        hdfs_namenode_port:
          storageType === 'HDFS' && !haEnabled
            ? Number(nameNodePort) || 0
            : undefined,
        hdfs_ha_enabled: storageType === 'HDFS' ? haEnabled : undefined,
        hdfs_name_services:
          storageType === 'HDFS' && haEnabled ? nameServices : undefined,
        hdfs_ha_namenodes:
          storageType === 'HDFS' && haEnabled ? haNamenodes : undefined,
        hdfs_namenode_rpc_address_1:
          storageType === 'HDFS' && haEnabled ? rpc1 : undefined,
        hdfs_namenode_rpc_address_2:
          storageType === 'HDFS' && haEnabled ? rpc2 : undefined,
        kerberos_principal:
          storageType === 'HDFS' && kerberos ? principal : undefined,
        kerberos_keytab_file_path:
          storageType === 'HDFS' && kerberos ? keytab : undefined,
        hdfs_site_path: storageType === 'HDFS' ? hdfsSitePath : undefined,
        s3_credentials_provider:
          storageType === 'S3' ? s3Provider : undefined,
      },
    );
    setSaving(false);
    if (!result.success || !result.data) {
      const msg = result.error || t('cluster.runtimeStorage.saveFailed');
      setError(msg);
      toast.error(msg);
      return;
    }
    if (!result.data.saved) {
      const hostMessage = result.data.validation?.hosts?.find(
        (host) => !host.success,
      )?.message;
      const msg =
        hostMessage ||
        result.data.message ||
        t('cluster.runtimeStorage.testFailed');
      setError(msg);
      toast.error(msg);
      return;
    }
    const versions = (result.data.versions || [])
      .map((item) => `${item.config_type} v${item.version}`)
      .join(', ');
    const successMessage = t('cluster.runtimeStorage.savedRestart', {
      versions: versions || '-',
    });
    preserveNoticeRef.current = true;
    setNotice(successMessage);
    toast.success(successMessage);
    setAccessKey('');
    setSecretKey('');
    onApplied?.();
  };

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div>
          <div className='text-sm font-semibold text-foreground'>
            {t('cluster.runtimeStorage.editTitle')}
          </div>
          <p className='text-xs text-muted-foreground'>
            {t('cluster.runtimeStorage.editHint')}
          </p>
        </div>
        {kind === 'imap' ? (
          <Switch
            id={`${kind}-enabled`}
            label={t('cluster.runtimeStorage.enabled')}
            checked={enabled}
            onCheckedChange={setEnabled}
          />
        ) : null}
      </div>

      {kind === 'checkpoint' || enabled ? (
        <div className='grid gap-4 md:grid-cols-2'>
          <Select
            label={t('cluster.runtimeStorage.storageType')}
            value={storageType}
            onValueChange={setStorageType}
            options={STORAGE_TYPE_OPTIONS}
          />
          <Input
            label={t('cluster.runtimeStorage.path')}
            value={namespace}
            onChange={(event) => setNamespace(event.target.value)}
            className='font-mono text-xs'
          />

          {storageType === 'HDFS' ? (
            <div className='space-y-3 md:col-span-2'>
              <Checkbox
                label={t('installer.hdfsHAMode')}
                checked={haEnabled}
                onCheckedChange={(checked) => setHaEnabled(checked === true)}
              />
              {!haEnabled ? (
                <div className='grid gap-4 md:grid-cols-2'>
                  <Field
                    label={t('installer.hdfsNameNodeHost')}
                    value={nameNodeHost}
                    onChange={setNameNodeHost}
                    placeholder='namenode.example.com'
                  />
                  <Field
                    label={t('installer.hdfsNameNodePort')}
                    value={nameNodePort}
                    onChange={setNameNodePort}
                    placeholder='8020'
                  />
                </div>
              ) : (
                <div className='grid gap-4 md:grid-cols-2'>
                  <Field
                    label={t('installer.hdfsNameServices')}
                    value={nameServices}
                    onChange={setNameServices}
                    placeholder='usdp-bing'
                  />
                  <Field
                    label={t('installer.hdfsHANamenodes')}
                    value={haNamenodes}
                    onChange={setHaNamenodes}
                    placeholder='nn1,nn2'
                  />
                  <Field
                    label={t('installer.hdfsNamenodeRPCAddress1')}
                    value={rpc1}
                    onChange={setRpc1}
                    placeholder='nn1-host:8020'
                  />
                  <Field
                    label={t('installer.hdfsNamenodeRPCAddress2')}
                    value={rpc2}
                    onChange={setRpc2}
                    placeholder='nn2-host:8020'
                  />
                </div>
              )}
              <Checkbox
                label={t('installer.hdfsKerberos')}
                checked={kerberos}
                onCheckedChange={(checked) => setKerberos(checked === true)}
              />
              {kerberos ? (
                <div className='grid gap-4 md:grid-cols-2'>
                  <Field
                    label={t('installer.kerberosPrincipal')}
                    value={principal}
                    onChange={setPrincipal}
                    placeholder='hdfs/nn@EXAMPLE.COM'
                  />
                  <Field
                    label={t('installer.kerberosKeytabPath')}
                    value={keytab}
                    onChange={setKeytab}
                    placeholder='/etc/security/keytabs/hdfs.keytab'
                  />
                </div>
              ) : null}
              <Field
                label={t('installer.hdfsSitePath')}
                value={hdfsSitePath}
                onChange={setHdfsSitePath}
                placeholder='/etc/hadoop/conf/hdfs-site.xml'
              />
            </div>
          ) : null}

          {storageType === 'OSS' ? (
            <>
              <Field
                label={t('installer.endpoint')}
                value={endpoint}
                onChange={setEndpoint}
                placeholder='oss-cn-hangzhou.aliyuncs.com'
              />
              <Field
                label={t('installer.bucket')}
                value={bucket}
                onChange={setBucket}
                placeholder='your-bucket'
              />
              <Field
                label='fs.oss.accessKeyId'
                value={accessKey}
                onChange={setAccessKey}
                placeholder={t('cluster.runtimeStorage.keepSecret')}
                secret
              />
              <Field
                label='fs.oss.accessKeySecret'
                value={secretKey}
                onChange={setSecretKey}
                placeholder={t('cluster.runtimeStorage.keepSecret')}
                secret
              />
            </>
          ) : null}

          {storageType === 'S3' ? (
            <>
              <Select
                label={t('installer.s3CredentialsProvider')}
                value={s3Provider}
                onValueChange={setS3Provider}
                options={[
                  {value: S3_SIMPLE, label: t('installer.s3ProviderSimple')},
                  {value: S3_INSTANCE, label: t('installer.s3ProviderInstance')},
                ]}
              />
              <Field
                label={t('installer.bucket')}
                value={bucket}
                onChange={setBucket}
                placeholder='s3a://bucket'
              />
              <Field
                label={t('installer.endpoint')}
                value={endpoint}
                onChange={setEndpoint}
                placeholder='http://127.0.0.1:9000'
              />
              {s3Provider !== S3_INSTANCE ? (
                <>
                  <Field
                    label='fs.s3a.access.key'
                    value={accessKey}
                    onChange={setAccessKey}
                    placeholder={t('cluster.runtimeStorage.keepSecret')}
                    secret
                  />
                  <Field
                    label='fs.s3a.secret.key'
                    value={secretKey}
                    onChange={setSecretKey}
                    placeholder={t('cluster.runtimeStorage.keepSecret')}
                    secret
                  />
                </>
              ) : null}
            </>
          ) : null}
        </div>
      ) : null}

      {error ? <ArcAlert tone='danger' title={error} /> : null}
      {notice ? <ArcAlert tone='success' title={notice} /> : null}
      <div className='flex justify-end'>
        <Button
          type='button'
          size='sm'
          onClick={() => void handleSave()}
          loading={saving}
        >
          {t('cluster.runtimeStorage.saveAndTest')}
        </Button>
      </div>
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  placeholder,
  secret,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  secret?: boolean;
}) {
  return (
    <Input
      type={secret ? 'password' : 'text'}
      label={label}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      placeholder={placeholder}
      className='font-mono text-xs'
    />
  );
}
