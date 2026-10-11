import { useState } from "react";
import { createLogLibraryByILogtailK8s } from "../api/logLibrary";
import type { QuerySourceInstance } from "../../query/types/contracts";

interface TemplateILogtailK8sModalProps {
  open: boolean;
  instances: QuerySourceInstance[];
  onClose: () => void;
  onSuccess: () => void;
}

// Single-entry iLogtail K8s template creation. Mirrors the v1 "ilogtail_k8s"
// template payload { databaseId, name, brokers, topic, days } and posts to the
// root-only management façade route.
export function TemplateILogtailK8sModal({
  open,
  instances,
  onClose,
  onSuccess,
}: TemplateILogtailK8sModalProps) {
  const [databaseId, setDatabaseId] = useState(0);
  const [name, setName] = useState("");
  const [brokers, setBrokers] = useState("kafka:9092");
  const [topic, setTopic] = useState("");
  const [days, setDays] = useState(7);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit() {
    if (saving) return;
    if (
      !databaseId ||
      !name.trim() ||
      !brokers.trim() ||
      !topic.trim() ||
      !days ||
      days <= 0
    ) {
      setError("请填写数据库、表名、Brokers、Topic 与有效的 TTL 天数");
      return;
    }
    setSaving(true);
    setError("");
    try {
      await createLogLibraryByILogtailK8s({
        databaseId,
        name: name.trim(),
        brokers: brokers.trim(),
        topic: topic.trim(),
        days,
      });
      onSuccess();
      onClose();
    } catch (saveError) {
      setError(
        saveError instanceof Error ? saveError.message : "iLogtail K8s 接入失败",
      );
    } finally {
      setSaving(false);
    }
  }

  if (!open) {
    return null;
  }

  return (
    <div
      className="cv-report-modal-backdrop cv-log-library-create-backdrop"
      role="presentation"
    >
      <section
        className="cv-report-modal cv-log-library-create"
        role="dialog"
        aria-modal="true"
        aria-labelledby="cv-ilogtail-k8s-title"
      >
        <div className="cv-log-library-create__header">
          <div>
            <div className="cv-settings-section-eyebrow">LOG LIBRARY</div>
            <h2 className="cv-panel-title" id="cv-ilogtail-k8s-title">
              iLogtail K8s 接入
            </h2>
            <p className="cv-log-library-create__description">
              使用 iLogtail K8s 模板快速创建 Kafka 采集日志库。
            </p>
          </div>
          <button
            type="button"
            className="cv-icon-button cv-log-library-create__close"
            onClick={onClose}
            aria-label="关闭 iLogtail K8s 接入"
            title="关闭"
          >
            ×
          </button>
        </div>

        <div className="cv-log-library-create__body">
          <label className="cv-log-library-field">
            <span className="cv-log-library-field__label">
              数据库 <em>必填</em>
            </span>
            <select
              className="cv-select cv-log-library-field__control"
              aria-label="数据库"
              value={databaseId}
              onChange={(event) => setDatabaseId(Number(event.target.value))}
            >
              <option value={0}>选择实例 / 数据库</option>
              {instances.flatMap((instance) =>
                instance.databases.map((database) => (
                  <option key={database.id} value={database.id}>
                    {instance.name} / {database.name}
                  </option>
                )),
              )}
            </select>
          </label>

          <label className="cv-log-library-field">
            <span className="cv-log-library-field__label">
              日志表名 <em>必填</em>
            </span>
            <input
              className="cv-text-input cv-log-library-field__control"
              aria-label="日志表名"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="例如：k8s_logs"
            />
          </label>

          <div className="cv-log-library-create__grid">
            <label className="cv-log-library-field">
              <span className="cv-log-library-field__label">Kafka Brokers</span>
              <input
                className="cv-text-input cv-log-library-field__control"
                aria-label="Kafka Brokers"
                value={brokers}
                onChange={(event) => setBrokers(event.target.value)}
                placeholder="kafka:9092"
              />
            </label>
            <label className="cv-log-library-field">
              <span className="cv-log-library-field__label">Kafka Topic</span>
              <input
                className="cv-text-input cv-log-library-field__control"
                aria-label="Kafka Topic"
                value={topic}
                onChange={(event) => setTopic(event.target.value)}
                placeholder="kafka topic"
              />
            </label>
          </div>

          <label className="cv-log-library-field">
            <span className="cv-log-library-field__label">TTL 天数</span>
            <input
              className="cv-text-input cv-log-library-field__control"
              aria-label="TTL 天数"
              type="number"
              min={1}
              value={days || ""}
              onChange={(event) => setDays(Number(event.target.value))}
            />
          </label>

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
            disabled={saving}
          >
            {saving ? "接入中..." : "确认接入"}
          </button>
        </div>
      </section>
    </div>
  );
}

export default TemplateILogtailK8sModal;
