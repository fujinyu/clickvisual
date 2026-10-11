import { useEffect, useState } from "react";
import {
  getLogLibraryDetail,
  updateLogLibrary,
  type LogLibraryDetail,
} from "../api/logLibrary";

interface EditLogLibraryModalProps {
  open: boolean;
  tableId: number;
  onClose: () => void;
  onSuccess: () => void;
}

// Edit an existing log library. Only the retention days and (when the table has
// a cold volume) the hot days are editable; storage_policy / cold_volume are
// frozen after creation. The kafka fields are echoed back so the PATCH does not
// wipe them.
export function EditLogLibraryModal({
  open,
  tableId,
  onClose,
  onSuccess,
}: EditLogLibraryModalProps) {
  const [detail, setDetail] = useState<LogLibraryDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [days, setDays] = useState(0);
  const [hotDays, setHotDays] = useState(0);
  const [desc, setDesc] = useState("");

  useEffect(() => {
    if (!open || !tableId) {
      setDetail(null);
      return;
    }
    let active = true;
    setLoading(true);
    setError("");
    getLogLibraryDetail(tableId)
      .then((data) => {
        if (!active) return;
        setDetail(data);
        setDays(data.days || 0);
        setHotDays(data.hotDays || 0);
        setDesc(data.desc || "");
      })
      .catch((loadError) => {
        if (active) {
          setError(
            loadError instanceof Error ? loadError.message : "日志库详情加载失败",
          );
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [open, tableId]);

  async function handleSubmit() {
    if (!detail || saving) return;
    if (!days || days <= 0) {
      setError("数据保留天数需为正整数");
      return;
    }
    const hasCold = Boolean(detail.coldVolume);
    if (hasCold && (hotDays <= 0 || hotDays >= days)) {
      setError("热数据天数需为小于总保留天数的正整数");
      return;
    }
    setSaving(true);
    setError("");
    try {
      await updateLogLibrary(tableId, {
        mergeTreeTTL: days,
        kafkaBrokers: detail.brokers,
        kafkaTopic: detail.topic,
        kafkaConsumerNum: detail.consumerNum,
        kafkaSkipBrokenMessages: detail.kafkaSkipBrokenMessages,
        desc,
        v3TableType: detail.v3TableType,
        hotDays: hasCold ? hotDays : 0,
      });
      onSuccess();
      onClose();
    } catch (saveError) {
      setError(
        saveError instanceof Error ? saveError.message : "日志库更新失败",
      );
    } finally {
      setSaving(false);
    }
  }

  if (!open || !tableId) {
    return null;
  }

  const hasCold = Boolean(detail?.coldVolume);

  return (
    <div
      className="cv-report-modal-backdrop cv-log-library-create-backdrop"
      role="presentation"
    >
      <section
        className="cv-report-modal cv-log-library-create"
        role="dialog"
        aria-modal="true"
        aria-labelledby="cv-log-library-edit-title"
      >
        <div className="cv-log-library-create__header">
          <div>
            <div className="cv-settings-section-eyebrow">LOG LIBRARY</div>
            <h2 className="cv-panel-title" id="cv-log-library-edit-title">
              编辑日志库
            </h2>
            <p className="cv-log-library-create__description">
              调整数据保留天数与冷热分层的热数据天数。
            </p>
          </div>
          <button
            type="button"
            className="cv-icon-button cv-log-library-create__close"
            onClick={onClose}
            aria-label="关闭编辑日志库"
            title="关闭"
          >
            ×
          </button>
        </div>

        <div className="cv-log-library-create__body">
          {loading ? (
            <div className="cv-settings-empty">加载中...</div>
          ) : detail ? (
            <>
              <label className="cv-log-library-field">
                <span className="cv-log-library-field__label">数据保留天数</span>
                <input
                  className="cv-text-input cv-log-library-field__control"
                  aria-label="数据保留天数"
                  type="number"
                  min={1}
                  value={days || ""}
                  onChange={(event) => setDays(Number(event.target.value))}
                />
              </label>
              {detail.storagePolicy || detail.coldVolume ? (
                <div className="cv-log-library-field">
                  <span className="cv-log-library-field__label">
                    冷热分层（只读）
                  </span>
                  <span className="cv-muted">
                    存储策略：{detail.storagePolicy || "-"} · 冷数据 Volume：
                    {detail.coldVolume || "-"}
                  </span>
                </div>
              ) : null}
              {hasCold ? (
                <label className="cv-log-library-field">
                  <span className="cv-log-library-field__label">热数据天数</span>
                  <input
                    className="cv-text-input cv-log-library-field__control"
                    aria-label="热数据天数"
                    type="number"
                    min={1}
                    value={hotDays || ""}
                    onChange={(event) => setHotDays(Number(event.target.value))}
                    placeholder="小于总保留天数"
                  />
                </label>
              ) : null}
              <label className="cv-log-library-field">
                <span className="cv-log-library-field__label">说明</span>
                <textarea
                  className="cv-text-input cv-log-library-field__control"
                  aria-label="说明"
                  value={desc}
                  onChange={(event) => setDesc(event.target.value)}
                  rows={3}
                />
              </label>
            </>
          ) : null}
          {error ? (
            <div className="cv-settings-banner cv-settings-banner--error">
              <span>{error}</span>
            </div>
          ) : null}
        </div>

        <div className="cv-log-library-create__footer">
          <button
            type="button"
            className="cv-secondary-button"
            onClick={onClose}
          >
            取消
          </button>
          <button
            type="button"
            className="cv-action-button"
            onClick={() => void handleSubmit()}
            disabled={saving || loading || !detail}
          >
            {saving ? "保存中..." : "保存"}
          </button>
        </div>
      </section>
    </div>
  );
}

export default EditLogLibraryModal;
