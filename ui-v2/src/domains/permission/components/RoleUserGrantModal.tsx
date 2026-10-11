import { useEffect, useMemo, useState } from "react";
import {
  getPermissionRoleUids,
  grantPermissionRoleUids,
  listPermissionUsers,
  type PermissionUser
} from "../api/permission";
import { EmptyState, LoadingState } from "../../../shared/state/PageState";

const USER_PAGE_SIZE = 200;

async function loadAllPermissionUsers() {
  const records: PermissionUser[] = [];
  let current = 1;
  let total = 0;

  do {
    const response = await listPermissionUsers({
      current,
      pageSize: USER_PAGE_SIZE
    });
    records.push(...response.list);
    total = response.total;
    current += 1;
  } while (records.length < total);

  return records;
}

interface RoleUserGrantModalProps {
  roleId: number;
  roleName: string;
  onClose: () => void;
  onGranted: () => void;
}

// Role-user association. Mirrors the v1 RoleUserForm contract: GET the current
// members via /pms/role/uids/{id}, edit the multi-select, then POST the full
// root_uids list to /pms/role/grant/{id} (submit overwrites the roster).
export default function RoleUserGrantModal({
  roleId,
  roleName,
  onClose,
  onGranted
}: RoleUserGrantModalProps) {
  const [users, setUsers] = useState<PermissionUser[]>([]);
  const [selectedUids, setSelectedUids] = useState<number[]>([]);
  const [keyword, setKeyword] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    let active = true;

    async function bootstrap() {
      setLoading(true);
      setErrorMessage(null);
      try {
        const [allUsers, roleUids] = await Promise.all([
          loadAllPermissionUsers(),
          getPermissionRoleUids(roleId)
        ]);
        if (!active) {
          return;
        }
        setUsers(allUsers);
        setSelectedUids(roleUids.root_uids ?? []);
      } catch (error) {
        if (active) {
          setErrorMessage(
            error instanceof Error ? error.message : "关联用户加载失败"
          );
        }
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    }

    void bootstrap();
    return () => {
      active = false;
    };
  }, [roleId]);

  const filteredUsers = useMemo(() => {
    const normalized = keyword.trim().toLowerCase();
    if (!normalized) {
      return users;
    }
    return users.filter(
      (user) =>
        user.username.toLowerCase().includes(normalized) ||
        user.nickname.toLowerCase().includes(normalized)
    );
  }, [keyword, users]);

  function toggleUid(uid: number) {
    setSelectedUids((current) =>
      current.includes(uid)
        ? current.filter((item) => item !== uid)
        : current.concat(uid)
    );
  }

  async function handleSave() {
    if (saving) {
      return;
    }
    setSaving(true);
    setErrorMessage(null);
    try {
      await grantPermissionRoleUids(roleId, { root_uids: selectedUids });
      onGranted();
      onClose();
    } catch (error) {
      setErrorMessage(
        error instanceof Error ? error.message : "关联用户保存失败"
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="cv-report-modal-backdrop" role="presentation">
      <section
        className="cv-report-modal cv-permission-root-modal"
        role="dialog"
        aria-label={`关联用户 ${roleName}`}
      >
        <div className="cv-panel-header cv-permission-root-modal__header">
          <div>
            <h2 className="cv-panel-title">关联用户</h2>
            <p className="cv-panel-description">
              为角色 {roleName} 选择用户，保存后覆盖当前名单。
            </p>
          </div>
          <div className="cv-permission-root-modal__selection" aria-live="polite">
            <span className="cv-label">已选</span>
            <strong>{selectedUids.length}</strong>
          </div>
        </div>

        {loading ? (
          <LoadingState title="用户加载中" description="正在获取用户与当前关联。" />
        ) : null}

        {!loading && errorMessage ? (
          <div className="cv-status-card" role="alert">
            {errorMessage}
          </div>
        ) : null}

        {!loading ? (
          <>
            <div className="cv-permission-root-modal__toolbar">
              <label className="cv-form-row cv-permission-search">
                <span className="cv-label">搜索用户</span>
                <input
                  aria-label="搜索关联用户"
                  className="cv-input"
                  value={keyword}
                  onChange={(event) => setKeyword(event.target.value)}
                  placeholder="按用户名或昵称搜索用户"
                />
              </label>
            </div>

            {filteredUsers.length > 0 ? (
              <div className="cv-permission-root-modal__list" role="list">
                {filteredUsers.map((user) => {
                  const isSelected = selectedUids.includes(user.uid);
                  return (
                    <button
                      key={user.uid}
                      type="button"
                      className={`cv-permission-user-picker cv-permission-root-modal__row${
                        isSelected ? " cv-permission-user-picker--selected" : ""
                      }`}
                      aria-label={`选择用户 ${user.nickname || user.username}`}
                      aria-pressed={isSelected}
                      onClick={() => toggleUid(user.uid)}
                    >
                      <div className="cv-permission-root-modal__identity">
                        <div className="cv-permission-item-title-row">
                          <strong>{user.nickname || user.username}</strong>
                          <span className="cv-permission-inline-chip">UID {user.uid}</span>
                        </div>
                        <div className="cv-muted">{user.email || "暂无邮箱"}</div>
                      </div>
                      <span className="cv-permission-root-modal__username">{user.username}</span>
                      <span className="cv-permission-root-modal__state">
                        {isSelected ? "已关联" : "未关联"}
                      </span>
                    </button>
                  );
                })}
              </div>
            ) : (
              <EmptyState title="未找到匹配用户" description="请尝试其他用户名或昵称关键字。" />
            )}
          </>
        ) : null}

        <div className="cv-header-actions">
          <button type="button" className="cv-secondary-button" onClick={onClose}>
            取消
          </button>
          <button
            type="button"
            className="cv-action-button"
            disabled={saving || loading}
            onClick={() => void handleSave()}
          >
            {saving ? "保存中..." : "保存关联"}
          </button>
        </div>
      </section>
    </div>
  );
}
