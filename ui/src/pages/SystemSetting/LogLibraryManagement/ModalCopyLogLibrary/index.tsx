import api, { LogLibraryCopyResult } from "@/services/dataLogs";
import systemSettingApi from "@/services/systemSetting";
import {
  Alert,
  Button,
  Checkbox,
  Empty,
  message,
  Modal,
  Select,
  Spin,
  Table,
} from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useIntl } from "umi";
import styles from "./index.less";

interface CopyTableItem {
  id: number;
  tableName: string;
  desc?: string;
}

interface CopyDatabaseItem {
  id: number;
  databaseName?: string;
  name?: string;
  tables?: CopyTableItem[];
}

interface CopyInstanceItem {
  id: number;
  instanceName?: string;
  name?: string;
  databases?: CopyDatabaseItem[];
}

const ModalCopyLogLibrary = (props: {
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void;
}) => {
  const { open, onClose, onSuccess } = props;
  const i18n = useIntl();
  const t = (id: string, values?: any) => i18n.formatMessage({ id }, values);

  const [instances, setInstances] = useState<CopyInstanceItem[]>([]);
  const [loadingInstances, setLoadingInstances] = useState(false);
  const [sourceInstanceId, setSourceInstanceId] = useState<number>();
  const [sourceDatabaseId, setSourceDatabaseId] = useState<number>();
  const [targetInstanceId, setTargetInstanceId] = useState<number>();
  const [targetDatabaseId, setTargetDatabaseId] = useState<number>();
  const [selectedIds, setSelectedIds] = useState<number[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<LogLibraryCopyResult>();

  const instanceOptions = useMemo(
    () =>
      instances.map((item) => ({
        value: item.id,
        label: item.instanceName || item.name || "",
      })),
    [instances]
  );

  const databaseOptions = useCallback(
    (instanceId?: number) => {
      const instance = instances.find((item) => item.id === instanceId);
      return (instance?.databases || []).map((item) => ({
        value: item.id,
        label: item.databaseName || item.name || "",
      }));
    },
    [instances]
  );

  const sourceTableItems = useMemo(() => {
    const instance = instances.find((item) => item.id === sourceInstanceId);
    const database = (instance?.databases || []).find(
      (item) => item.id === sourceDatabaseId
    );
    return database?.tables || [];
  }, [instances, sourceInstanceId, sourceDatabaseId]);

  const allChecked =
    sourceTableItems.length > 0 &&
    selectedIds.length === sourceTableItems.length;
  const indeterminate =
    selectedIds.length > 0 && selectedIds.length < sourceTableItems.length;
  const sameDatabase =
    !!sourceDatabaseId &&
    !!targetDatabaseId &&
    sourceDatabaseId === targetDatabaseId;

  useEffect(() => {
    if (!open) {
      setSourceInstanceId(undefined);
      setSourceDatabaseId(undefined);
      setTargetInstanceId(undefined);
      setTargetDatabaseId(undefined);
      setSelectedIds([]);
      setResult(undefined);
      return;
    }
    let mounted = true;
    setLoadingInstances(true);
    systemSettingApi
      .getAllInstances()
      .then((res: any) => {
        if (!mounted) return;
        if (res?.code !== 0) {
          message.error(res?.msg || t("logLibraryManagement.copy.loadError"));
          return;
        }
        setInstances(res?.data || []);
      })
      .catch(() => {
        if (mounted) message.error(t("logLibraryManagement.copy.loadError"));
      })
      .finally(() => {
        if (mounted) setLoadingInstances(false);
      });
    return () => {
      mounted = false;
    };
  }, [open]);

  const onConfirm = async () => {
    if (
      !sourceInstanceId ||
      !sourceDatabaseId ||
      !targetInstanceId ||
      !targetDatabaseId
    ) {
      message.warning(t("logLibraryManagement.copy.needSelection"));
      return;
    }
    if (sameDatabase) {
      message.warning(t("logLibraryManagement.copy.sameDatabase"));
      return;
    }
    if (selectedIds.length === 0) {
      message.warning(t("logLibraryManagement.copy.needSelection"));
      return;
    }
    setSubmitting(true);
    try {
      const res: any = await api.copyLogLibrary({
        sourceInstanceId,
        sourceDatabaseId,
        targetInstanceId,
        targetDatabaseId,
        tableIds: selectedIds,
      });
      if (res?.code !== 0) {
        message.error(res?.msg || t("logLibraryManagement.copy.error"));
        return;
      }
      setResult(res?.data);
      onSuccess?.();
    } catch (error) {
      message.error(t("logLibraryManagement.copy.error"));
    } finally {
      setSubmitting(false);
    }
  };

  const footer = result ? (
    <Button type="primary" onClick={onClose}>
      {t("logLibraryManagement.copy.done")}
    </Button>
  ) : (
    <>
      <Button onClick={onClose}>{t("button.cancel")}</Button>
      <Button
        type="primary"
        loading={submitting}
        disabled={
          !sourceInstanceId ||
          !sourceDatabaseId ||
          !targetInstanceId ||
          !targetDatabaseId ||
          sameDatabase ||
          selectedIds.length === 0
        }
        onClick={onConfirm}
      >
        {t("logLibraryManagement.copy.confirm")}
      </Button>
    </>
  );

  return (
    <Modal
      open={open}
      title={t("logLibraryManagement.copy.title")}
      width={680}
      footer={footer}
      onCancel={() => {
        if (!submitting) onClose();
      }}
      destroyOnClose
    >
      <Spin spinning={loadingInstances}>
        {result && (
          <Alert
            style={{ marginBottom: 12 }}
            type={result.failedCount > 0 ? "warning" : "success"}
            showIcon
            message={
              <div className={styles.resultMessage}>
                <span>
                  {t("logLibraryManagement.copy.resultSuccess", {
                    copied: result.copied,
                    total: result.total,
                  })}
                </span>
                {result.skipped.length > 0 && (
                  <span>
                    {t("logLibraryManagement.copy.resultSkipped", {
                      count: result.skipped.length,
                    })}
                  </span>
                )}
                {result.failedCount > 0 && (
                  <span className={styles.resultFailed}>
                    {t("logLibraryManagement.copy.resultFailed", {
                      count: result.failedCount,
                    })}
                  </span>
                )}
              </div>
            }
            description={
              result.failed.length > 0 ? (
                <div className={styles.resultFailedList}>
                  <div>{t("logLibraryManagement.copy.failedList")}</div>
                  <div>{result.failed.join("、")}</div>
                </div>
              ) : undefined
            }
          />
        )}
        <div className={styles.section}>
          <div className={styles.sectionTitle}>
            {t("logLibraryManagement.copy.source")}
          </div>
          <div className={styles.fields}>
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder={t("logLibraryManagement.copy.sourceInstance")}
              value={sourceInstanceId}
              onChange={(value) => {
                setSourceInstanceId(value);
                setSourceDatabaseId(undefined);
                setSelectedIds([]);
              }}
              options={instanceOptions}
            />
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              disabled={!sourceInstanceId}
              placeholder={t("logLibraryManagement.copy.sourceDatabase")}
              value={sourceDatabaseId}
              onChange={(value) => {
                setSourceDatabaseId(value);
                setSelectedIds([]);
              }}
              options={databaseOptions(sourceInstanceId)}
            />
          </div>
        </div>
        <div className={styles.section}>
          <div className={styles.sectionTitle}>
            {t("logLibraryManagement.copy.target")}
          </div>
          <div className={styles.fields}>
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder={t("logLibraryManagement.copy.targetInstance")}
              value={targetInstanceId}
              onChange={(value) => {
                setTargetInstanceId(value);
                setTargetDatabaseId(undefined);
              }}
              options={instanceOptions}
            />
            <Select
              allowClear
              showSearch
              optionFilterProp="label"
              disabled={!targetInstanceId}
              placeholder={t("logLibraryManagement.copy.targetDatabase")}
              value={targetDatabaseId}
              onChange={setTargetDatabaseId}
              options={databaseOptions(targetInstanceId)}
            />
          </div>
          {sameDatabase && (
            <div className={styles.sameDatabaseHint}>
              {t("logLibraryManagement.copy.sameDatabase")}
            </div>
          )}
        </div>
        <div className={styles.section}>
          <div className={styles.tableHeader}>
            <Checkbox
              indeterminate={indeterminate}
              checked={allChecked}
              disabled={sourceTableItems.length === 0}
              onChange={(event) =>
                setSelectedIds(
                  event.target.checked
                    ? sourceTableItems.map((item) => item.id)
                    : []
                )
              }
            >
              {t("logLibraryManagement.copy.selectAll")}
            </Checkbox>
            <span className={styles.selectedCount}>
              {t("logLibraryManagement.copy.selected", {
                count: selectedIds.length,
              })}
            </span>
          </div>
          {sourceTableItems.length === 0 ? (
            <div className={styles.empty}>
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={t("logLibraryManagement.copy.noTables")}
              />
            </div>
          ) : (
            <Table<CopyTableItem>
              rowKey="id"
              size="small"
              pagination={false}
              scroll={{ y: 240 }}
              dataSource={sourceTableItems}
              rowSelection={{
                selectedRowKeys: selectedIds,
                onChange: (keys) => setSelectedIds(keys as number[]),
              }}
              columns={[
                {
                  title: t("logLibraryManagement.table"),
                  dataIndex: "tableName",
                },
                {
                  title: t("description"),
                  dataIndex: "desc",
                  render: (value: string) => value || "-",
                },
              ]}
            />
          )}
        </div>
      </Spin>
    </Modal>
  );
};

export default ModalCopyLogLibrary;
