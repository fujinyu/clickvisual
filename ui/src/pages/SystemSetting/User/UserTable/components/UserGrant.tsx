import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";
import {
  Button,
  Drawer,
  Empty,
  Form,
  List,
  message,
  Modal,
  Popconfirm,
  Select,
  Space,
  Spin,
  Tag,
} from "antd";
import { useEffect, useMemo, useState } from "react";
import { useIntl, useModel } from "umi";
import { CascaderInfo } from "@/models/pms";
import { reqGetPmsGrant, reqUpdatePmsGrant } from "@/services/pms";
import type { PmsGrantRes } from "@/services/pms";
import api, { InstanceType } from "@/services/systemSetting";
import { userListType } from "../..";

export interface UserGrantAggregation {
  instanceId: number;
  instanceName: string;
  roleId: number;
  roleName: string;
  roleDesc: string;
  domain: string[];
  acts: string[];
  subResources: string[];
}

const UserGrant = ({
  open,
  user,
  onClose,
}: {
  open: boolean;
  user: userListType | null;
  onClose: () => void;
}) => {
  const i18n = useIntl();
  const { commonInfo, fetchPmsCommonInfo } = useModel("pms");
  const [instances, setInstances] = useState<InstanceType[]>([]);
  const [grants, setGrants] = useState<Record<number, PmsGrantRes>>({});
  const [loading, setLoading] = useState<boolean>(false);
  const [addVisible, setAddVisible] = useState<boolean>(false);
  const [addForm] = Form.useForm();
  const [addInstanceId, setAddInstanceId] = useState<number | null>(null);

  // 打开抽屉时加载实例列表与各实例的角色授权，并确保 commonInfo（数据域选项）已加载
  useEffect(() => {
    if (!open || !user) return;
    setLoading(true);
    if (!commonInfo) {
      fetchPmsCommonInfo(0);
    }
    (async () => {
      try {
        const instanceRes = await api.getInstances();
        if (instanceRes.code !== 0) {
          message.error(instanceRes.msg);
          return;
        }
        const nextInstances: InstanceType[] = instanceRes.data || [];
        setInstances(nextInstances);
        const entries = await Promise.all(
          nextInstances.map(async (instance) => {
            const grantRes = await reqGetPmsGrant(instance.id as number);
            if (grantRes.code !== 0) {
              throw new Error(grantRes.msg);
            }
            return [
              instance.id as number,
              grantRes.data,
            ] as const;
          })
        );
        setGrants(Object.fromEntries(entries));
      } catch (error) {
        message.error(
          error instanceof Error ? error.message : `${error as string}`
        );
      } finally {
        setLoading(false);
      }
    })();
  }, [open, user]);

  // 该用户在全部实例上的授权（与 v2 权限中心一致：前端拉全量实例授权后本地聚合）
  const userGrants = useMemo(() => {
    if (!user) return [];
    const result: UserGrantAggregation[] = [];
    Object.values(grants).forEach((instanceGrant) => {
      const instance = instances.find((item) => item.id === instanceGrant.iid);
      instanceGrant.roles.forEach((role) => {
        role.grant.forEach((item) => {
          if (item.userIds.includes(user.uid)) {
            result.push({
              instanceId: instanceGrant.iid,
              instanceName: instance?.name || `#${instanceGrant.iid}`,
              roleId: role.id,
              roleName: role.name,
              roleDesc: role.desc,
              domain: item.domain,
              acts: role.details.flatMap((detail) => detail.acts),
              subResources: role.details.flatMap(
                (detail) => detail.sub_resources
              ),
            });
          }
        });
      });
    });
    return result;
  }, [grants, instances, user]);

  const domainOptions = useMemo(() => {
    function flatten(
      items: CascaderInfo[],
      valuePath: string[] = [],
      labelPath: string[] = []
    ): { value: string; label: string }[] {
      return items.flatMap((item) => {
        const nextValuePath = valuePath.concat(item.value);
        const nextLabelPath = labelPath.concat(item.label);
        const current = {
          value: nextValuePath.join("__"),
          label: nextLabelPath.join(" / "),
        };
        return item.children && item.children.length > 0
          ? flatten(item.children, nextValuePath, nextLabelPath)
          : [current];
      });
    }

    return commonInfo?.domainCascader
      ? flatten(commonInfo.domainCascader)
      : [];
  }, [commonInfo]);

  // 后端 domainCascader 的首项固定为 { value: "*", label: "全部" }（见 pmsplugin.GetDomainCascaderOptions），
  // 即“不限制数据域”；仅当 commonInfo 尚未加载（拿不到后端选项）时，才合成一个 * 兜底，避免出现两个等价的“全部”
  const domainSelectOptions = useMemo(() => {
    if (domainOptions.length > 0) {
      return domainOptions;
    }
    return [
      {
        value: "*",
        label: i18n.formatMessage({ id: "sys.user.grant.allDomain" }),
      },
    ];
  }, [domainOptions]);

  const rolesOfAddInstance = useMemo(() => {
    if (addInstanceId === null) return [];
    return grants[addInstanceId]?.roles ?? [];
  }, [grants, addInstanceId]);

  const saveGrant = async (iid: number, payload: PmsGrantRes) => {
    const res = await reqUpdatePmsGrant(iid, payload);
    if (res.code !== 0) {
      message.error(res.msg);
      return;
    }
    message.success(i18n.formatMessage({ id: "sys.user.grant.saveSuccess" }));
    setGrants((current) => ({ ...current, [iid]: payload }));
  };

  const handleRemove = async (target: UserGrantAggregation) => {
    const instanceGrant = grants[target.instanceId];
    if (!instanceGrant || !user) return;
    const next: PmsGrantRes = {
      ...instanceGrant,
      roles: instanceGrant.roles.map((role) => {
        if (role.id !== target.roleId) return role;
        return {
          ...role,
          grant: role.grant
            .map((item) => ({
              ...item,
              userIds: item.userIds.filter((uid) => uid !== user.uid),
            }))
            .filter((item) => item.userIds.length > 0),
        };
      }),
    };
    await saveGrant(target.instanceId, next);
  };

  const openAddModal = () => {
    const firstInstance = instances[0];
    setAddInstanceId(firstInstance?.id ?? null);
    addForm.setFieldsValue({
      instanceId: firstInstance?.id ? String(firstInstance.id) : undefined,
      roleId: undefined,
      domain: "*",
    });
    setAddVisible(true);
  };

  const handleAddSubmit = async () => {
    let values: any;
    try {
      values = await addForm.validateFields();
    } catch {
      // 表单校验失败时 antd 会自动展示字段错误，这里仅阻止继续提交
      return;
    }
    const instanceId = Number(values.instanceId);
    const roleId = Number(values.roleId);
    const domain: string[] =
      values.domain === "*"
        ? ["*"]
        : String(values.domain)
            .split("__")
            .filter(Boolean);
    const instanceGrant = grants[instanceId];
    if (!instanceGrant || !user) return;
    const domainKey = domain.join("__");
    const next: PmsGrantRes = {
      ...instanceGrant,
      roles: instanceGrant.roles.map((role) => {
        if (role.id !== roleId) return role;
        const existed = role.grant.find(
          (item) => item.domain.join("__") === domainKey
        );
        if (existed) {
          if (existed.userIds.includes(user.uid)) return role;
          return {
            ...role,
            grant: role.grant.map((item) =>
              item === existed
                ? { ...item, userIds: [...item.userIds, user.uid] }
                : item
            ),
          };
        }
        return {
          ...role,
          grant: [...role.grant, { created: 0, domain, userIds: [user.uid] }],
        };
      }),
    };
    await saveGrant(instanceId, next);
    setAddVisible(false);
    addForm.resetFields();
  };

  return (
    <>
      <Drawer
        title={`${i18n.formatMessage({
          id: "sys.user.grant.title",
        })} - ${user?.nickname || ""}`}
        open={open}
        onClose={onClose}
        width={720}
        extra={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={openAddModal}
            disabled={instances.length === 0}
          >
            {i18n.formatMessage({ id: "sys.user.grant.add" })}
          </Button>
        }
      >
        <Spin spinning={loading}>
          {userGrants.length === 0 ? (
            <Empty
              description={i18n.formatMessage({ id: "sys.user.grant.empty" })}
            />
          ) : (
            <List
              dataSource={userGrants}
              renderItem={(item) => (
                <List.Item
                  key={`${item.instanceId}-${item.roleId}-${item.domain.join(
                    "__"
                  )}`}
                  actions={[
                    <Popconfirm
                      key="remove"
                      title={i18n.formatMessage({
                        id: "sys.user.grant.removeConfirm",
                      })}
                      okText={i18n.formatMessage({ id: "button.ok" })}
                      cancelText={i18n.formatMessage({ id: "button.cancel" })}
                      onConfirm={() => handleRemove(item)}
                    >
                      <Button
                        danger
                        size="small"
                        type="link"
                        icon={<DeleteOutlined />}
                      >
                        {i18n.formatMessage({ id: "sys.user.grant.remove" })}
                      </Button>
                    </Popconfirm>,
                  ]}
                >
                  <List.Item.Meta
                    title={
                      <Space size={4}>
                        <span>{item.instanceName}</span>
                        <Tag color="blue">{item.roleName}</Tag>
                      </Space>
                    }
                    description={
                      <div>
                        <div>
                          {i18n.formatMessage({ id: "sys.user.grant.scope" })}
                          {"："}
                          {item.domain.join(" / ")}
                        </div>
                        <div style={{ marginTop: 4 }}>
                          {item.subResources.map((subResource) => (
                            <Tag key={`sub-${subResource}`} color="#108ee9">
                              {subResource}
                            </Tag>
                          ))}
                          {item.acts.map((act) => (
                            <Tag key={`act-${act}`} color="#87d068">
                              {act}
                            </Tag>
                          ))}
                        </div>
                      </div>
                    }
                  />
                </List.Item>
              )}
            />
          )}
        </Spin>
      </Drawer>

      <Modal
        title={i18n.formatMessage({ id: "sys.user.grant.add" })}
        open={addVisible}
        onOk={() => handleAddSubmit()}
        onCancel={() => {
          setAddVisible(false);
          addForm.resetFields();
        }}
        okText={i18n.formatMessage({ id: "sys.user.grant.submit" })}
        cancelText={i18n.formatMessage({ id: "button.cancel" })}
      >
        <Form form={addForm} layout="vertical">
          <Form.Item
            name="instanceId"
            label={i18n.formatMessage({ id: "sys.user.grant.instance" })}
            rules={[
              {
                required: true,
                message: i18n.formatMessage({
                  id: "sys.user.grant.instance.placeholder",
                }),
              },
            ]}
          >
            <Select
              options={instances.map((instance) => ({
                value: String(instance.id),
                label: instance.name,
              }))}
              onChange={(value: string) => {
                setAddInstanceId(Number(value));
                addForm.setFieldsValue({ roleId: undefined });
              }}
            />
          </Form.Item>
          <Form.Item
            name="roleId"
            label={i18n.formatMessage({ id: "sys.user.grant.role" })}
            rules={[
              {
                required: true,
                message: i18n.formatMessage({
                  id: "sys.user.grant.role.placeholder",
                }),
              },
            ]}
          >
            <Select
              options={rolesOfAddInstance.map((role) => ({
                value: String(role.id),
                label: `${role.name}${role.desc ? ` | ${role.desc}` : ""}`,
              }))}
              notFoundContent={i18n.formatMessage({
                id: "sys.user.grant.role.empty",
              })}
            />
          </Form.Item>
          <Form.Item
            name="domain"
            label={i18n.formatMessage({ id: "sys.user.grant.domain" })}
            rules={[
              {
                required: true,
                message: i18n.formatMessage({
                  id: "sys.user.grant.domain.placeholder",
                }),
              },
            ]}
          >
            <Select
              options={domainSelectOptions}
            />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

export default UserGrant;
