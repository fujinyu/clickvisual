import useRequest from "@/hooks/useRequest/useRequest";
import ItemForm from "@/pages/SystemSetting/Role/components/ItemForm";
import RoleModel from "@/pages/SystemSetting/Role/components/RoleModel";
import RootUserForm from "@/pages/SystemSetting/Role/components/RootUserForm";
import RoleUserForm from "@/pages/SystemSetting/Role/components/RoleUserForm";
import SearchTable, {
  SearchTableInstance,
} from "@/pages/SystemSetting/Role/components/SearchTable";
import useRole from "@/pages/SystemSetting/Role/hooks/useRole";
import RoleStyle from "@/pages/SystemSetting/Role/styles/index.less";
import {
  CheckRoot,
  // reqCreatePmsDefaultRole,
  reqDeleteRole,
  reqGetRoleList,
  reqRoleUids,
} from "@/services/pms";
import sysUserApi from "@/services/systemUser";
import {
  ClearOutlined,
  DeleteOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
  PlusOutlined,
  QuestionCircleOutlined,
  SearchOutlined,
  UsergroupAddOutlined,
} from "@ant-design/icons";
import { useModel } from "@umijs/max";
import {
  Button,
  Divider,
  Form,
  Input,
  message,
  Modal,
  Select,
  Space,
  Tag,
  Tooltip,
} from "antd";
import { useEffect, useRef, useState } from "react";
import { useIntl } from "umi";

function PmsDefaultRoles() {
  const i18n = useIntl();
  const actionRef = useRef<SearchTableInstance>();
  const { commonInfo, fetchPmsCommonInfo } = useModel("pms");
  const { onChangeRoleModal, doGetPmsRole } = useModel("pms");
  const [load, setLoad] = useState<any>();
  const [createModalVisible, handleCreateModalVisible] =
    useState<boolean>(false);
  const [grantRootUserVisible, handleGrantRootUserVisible] =
    useState<boolean>(false);
  const [grantRoleUserVisible, handleGrantRoleUserVisible] =
      useState<boolean>(false);
  const [grantRoleId, handleGrantRoleId] =
      useState<number>(0);
  // 每个角色已关联的用户 uid 列表（key 为角色 id），以及 uid -> 用户显示名
  const [roleUsersMap, setRoleUsersMap] = useState<
    Record<number, number[]>
  >({});
  const [userNameMap, setUserNameMap] = useState<Record<number, string>>({});
  const [form] = Form.useForm();
  const { handleCreate, handleGrantUsers,handleRoleGrantUsers } = useRole();
  const deleteRole = useRequest(reqDeleteRole, {
    loadingText: {
      loading: undefined,
      done: i18n.formatMessage({
        id: "systemSetting.role.delete.success",
      }),
    },
    onSuccess: (res) => actionRef.current?.refresh(),
  });
  const callBackRefresh = () => {
    actionRef.current?.refresh();
  };

  const editorRole = (roleId: number) => {
    doGetPmsRole(roleId).then((res) => {
      load;
      if (res?.code === 0)
        onChangeRoleModal(true, 1, "global", callBackRefresh);
    });
  };

  // 拉取角色列表中各角色已关联的用户，用于「关联用户」列展示；失败时静默，不影响主列表
  const loadRoleUsers = async (list: any[]) => {
    if (!list || list.length === 0) return;
    try {
      const userRes = await sysUserApi.getUserList({
        current: 1,
        pageSize: 100,
      });
      if (userRes.code === 0) {
        const nextUserNameMap: Record<number, string> = {};
        (userRes.data?.list || []).forEach((user: any) => {
          nextUserNameMap[user.uid] = user.nickname || user.username;
        });
        setUserNameMap(nextUserNameMap);
      }
      const entries = await Promise.all(
        list.map(async (role) => {
          const r = await reqRoleUids(role.id);
          return [
            role.id,
            r.code === 0 ? r.data?.root_uids || [] : [],
          ] as const;
        })
      );
      setRoleUsersMap((prev) => ({
        ...prev,
        ...Object.fromEntries(entries),
      }));
    } catch {
      // 关联用户加载失败仅影响辅助列，不阻塞主流程
    }
  };

  const doDeleteRole = (roleId: number) => {
    deleteRole.run(roleId);
  };

  const deleteRoleConfirm = (record: any) => {
    Modal.confirm({
      title: i18n.formatMessage({
        id: "systemSetting.role.delete.title",
      }),
      content: i18n.formatMessage(
        {
          id: "systemSetting.role.delete.content",
        },
        { name: record.name }
      ),
      icon: <ExclamationCircleOutlined style={{ color: "red" }} />,
      onOk: () => doDeleteRole(record.id),
      okButtonProps: { danger: true },
      okText: i18n.formatMessage({
        id: "button.ok",
      }),
      cancelText: i18n.formatMessage({
        id: "button.cancel",
      }),
    });
  };

  useEffect(() => {
    fetchPmsCommonInfo(0);
  }, []);

  const columns = [
    {
      title: i18n.formatMessage({
        id: "systemSetting.role.table.name",
      }),
      dataIndex: "name",
      key: "name",
      width: 60,
    },
    {
      title: i18n.formatMessage({
        id: "systemSetting.role.table.desc",
      }),
      dataIndex: "desc",
      key: "desc",
      width: 200,
    },
    {
      title: i18n.formatMessage({
        id: "systemSetting.role.table.belongResource",
      }),
      dataIndex: "belongResource",
      key: "belongResource",
      width: 200,
      render: (belongResource: any) => (
        <>
          {belongResource}
          {belongResource === "instance" ? (
            <Tooltip
              title={i18n.formatMessage({
                id: "systemSetting.role.table.instanceGrantTip",
              })}
            >
              <QuestionCircleOutlined
                style={{ marginLeft: 4, color: "rgba(0, 0, 0, 0.45)" }}
              />
            </Tooltip>
          ) : null}
        </>
      ),
    },
    {
      title: i18n.formatMessage({
        id: "systemSetting.role.table.subResources",
      }),
      dataIndex: "details",
      key: "subResources",
      width: 100,
      render: (details: any, _: any) => (
        <div style={{ display: "flex" }}>
          {details?.map((subResources: any) => {
            return (
              <>
                {subResources.subResources.map((item: string) => {
                  return <Tag color="#108ee9">{item}</Tag>;
                })}
              </>
            );
          })}
        </div>
      ),
    },
    {
      title: i18n.formatMessage({
        id: "systemSetting.role.table.acts",
      }),
      dataIndex: "acts",
      key: "acts",
      width: 100,
      render: (details: any, _: any) => (
        <div style={{ display: "flex" }}>
          {details?.map((acts: any) => {
            return (
              <>
                {acts.acts.map((item: any) => {
                  return <Tag color="#87d068">{item}</Tag>;
                })}
              </>
            );
          })}
        </div>
      ),
    },
    {
      title: i18n.formatMessage({
        id: "systemSetting.role.table.relatedUsers",
      }),
      key: "relatedUsers",
      width: 160,
      render: (_: any, record: any) => {
        const uids = roleUsersMap[record.id];
        if (uids === undefined) {
          return <span style={{ color: "rgba(0, 0, 0, 0.45)" }}>-</span>;
        }
        if (uids.length === 0) {
          return (
            <span style={{ color: "rgba(0, 0, 0, 0.45)" }}>
              {i18n.formatMessage({
                id: "systemSetting.role.table.noRelatedUsers",
              })}
            </span>
          );
        }
        return (
          <a
            onClick={() => {
              CheckRoot().then((r) => {
                if (r.code !== 0) {
                  message.error(r.msg);
                  return;
                }
                handleGrantRoleId(record.id);
                handleGrantRoleUserVisible(true);
              });
            }}
          >
            <Space size={4} wrap>
              {uids.map((uid: number) => (
                <Tag key={uid} color="#108ee9">
                  {userNameMap[uid] || `UID:${uid}`}
                </Tag>
              ))}
            </Space>
          </a>
        );
      },
    },
    {
      title: i18n.formatMessage({
        id: "operation",
      }),
      key: "operating",
      valueType: "option",
      width: 100,
      fixed: "right" as "right",
      render: (_: any, record: any) => {
        return (
          <>
            <Divider type="vertical" />
            <Tooltip
              title={i18n.formatMessage({
                id: "edit",
              })}
            >
              <EditOutlined
                onClick={() => {
                  CheckRoot().then((r) => {
                    setLoad(
                      message.loading(
                        i18n.formatMessage({
                          id: "spin",
                        }),
                        0
                      )
                    );
                    if (r.code !== 0) {
                      message.error(r.msg);
                      return;
                    }
                    editorRole(record.id);
                  });
                }}
              />
            </Tooltip>
            <Divider type="vertical" />
            <Tooltip
              title={i18n.formatMessage({
                id: "systemSetting.role.table.relatedUsers",
              })}
            >
              <UsergroupAddOutlined
                onClick={() => {
                  CheckRoot().then((r) => {
                    if (r.code !== 0) {
                      message.error(r.msg);
                      return;
                    }
                    handleGrantRoleId(record.id);
                    handleGrantRoleUserVisible(true);
                  });
                }}
              />
            </Tooltip>
            <Divider type="vertical" />
            <Tooltip
              title={i18n.formatMessage({
                id: "delete",
              })}
            >
              <DeleteOutlined
                onClick={() => {
                  CheckRoot().then((r) => {
                    if (r.code !== 0) {
                      message.error(r.msg);
                      return;
                    }
                    deleteRoleConfirm(record);
                  });
                }}
              />
            </Tooltip>
          </>
        );
      },
    },
  ];
  return (
    <div className={RoleStyle.roleMain}>
      <SearchTable
        pagination={false}
        ref={actionRef}
        request={(params) => {
          const name = params.name || "";
          const belongResource = params.belongResource || "";
          return reqGetRoleList(name, belongResource).then((r) => {
            if (r.code !== 0) {
              message.error(`${r.msg}`);
              return [];
            }
            loadRoleUsers(r.data || []);
            return r;
          });
        }}
        columns={columns}
        form={form}
        rowKey={(record: any) => record.id}
        formContent={(search, form) => {
          return (
            <div>
              <Form
                form={form}
                onFinish={(fields) => search(fields)}
                layout="inline"
              >
                <Form.Item
                  label={i18n.formatMessage({
                    id: "systemSetting.role.filtrate.label.belongResource",
                  })}
                  name="belongResource"
                >
                  <Select
                    showSearch
                    optionFilterProp="children"
                    style={{ width: 200 }}
                  >
                    {(commonInfo?.prefixes_info || []).map((item, index) => {
                      return (
                        <Select.Option key={index} value={item.name}>
                          {item.name} | {item.desc}
                        </Select.Option>
                      );
                    })}
                  </Select>
                </Form.Item>
                <Form.Item
                  label={i18n.formatMessage({
                    id: "systemSetting.role.filtrate.label.name",
                  })}
                  name="name"
                >
                  <Input style={{ width: 200 }} />
                </Form.Item>
                <Form.Item>
                  <Button htmlType="submit" type={"primary"}>
                    <SearchOutlined />
                    {i18n.formatMessage({
                      id: "search",
                    })}
                  </Button>
                </Form.Item>
                <Form.Item>
                  <Button
                    onClick={() => {
                      actionRef.current?.form.resetFields();
                      actionRef.current?.form.submit();
                    }}
                  >
                    <ClearOutlined />
                    {i18n.formatMessage({
                      id: "systemSetting.role.filtrate.clear",
                    })}
                  </Button>
                </Form.Item>
                <Form.Item>
                  <Button
                    type="primary"
                    onClick={() => {
                      CheckRoot().then((r) => {
                        if (r.code !== 0) {
                          message.error(r.msg);
                          return;
                        }
                        onChangeRoleModal(true, 1, "global", callBackRefresh);
                      });
                    }}
                  >
                    <PlusOutlined />{" "}
                    {i18n.formatMessage({
                      id: "create",
                    })}
                  </Button>
                </Form.Item>
                <Form.Item>
                  <Button
                    type="primary"
                    onClick={() => {
                      CheckRoot().then((r) => {
                        if (r.code !== 0) {
                          message.error(r.msg);
                          return;
                        }
                        handleGrantRootUserVisible(true);
                      });
                    }}
                  ><PlusOutlined />
                    {i18n.formatMessage({
                      id: "systemSetting.role.filtrate.rootAuthority",
                    })}
                  </Button>

                </Form.Item>
              </Form>
            </div>
          );
        }}
      />
      <ItemForm
        formTitle={i18n.formatMessage({
          id: "systemSetting.role.filtrate.createDefaultRole",
        })}
        onSubmit={async (value: any) => {
          const success = handleCreate(value);
          if (await success) {
            handleCreateModalVisible(false);
            if (actionRef.current) {
              actionRef.current.refresh();
            }
          }
        }}
        onCancel={() => handleCreateModalVisible(false)}
        modalVisible={createModalVisible}
      />
      <RootUserForm
        formTitle={i18n.formatMessage({
          id: "systemSetting.role.filtrate.superAdministratorAuthorization",
        })}
        onSubmit={async (value: any) => {
          const success = handleGrantUsers(value);
          if (await success) {
            handleGrantRootUserVisible(false);
            if (actionRef.current) {
              actionRef.current.refresh();
              location.reload();
            }
          }
        }}
        onCancel={() => handleGrantRootUserVisible(false)}
        modalVisible={grantRootUserVisible}
      />
      <RoleUserForm
          formTitle={i18n.formatMessage({
            id: "systemSetting.role.filtrate.roleAuthorization",
          })}
          onSubmit={async (value: any) => {
            var roleId=grantRoleId;
            const success = handleRoleGrantUsers(roleId,value);
            if (await success) {
              handleGrantRoleUserVisible(false);
              if (actionRef.current) {
                actionRef.current.refresh();
              }
            }
          }}
          onCancel={() => handleGrantRoleUserVisible(false)}
          roleId={grantRoleId}
          modalVisible={grantRoleUserVisible}
      />
      <RoleModel />
    </div>
  );
}

export default PmsDefaultRoles;
