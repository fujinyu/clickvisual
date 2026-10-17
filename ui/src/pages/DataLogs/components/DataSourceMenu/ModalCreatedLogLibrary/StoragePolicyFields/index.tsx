import { Form, InputNumber, Select } from "antd";
import { useEffect, useMemo, useState } from "react";
import { useIntl, useModel } from "umi";
import type { StoragePolicyResponse } from "@/services/dataLogs";

const { Option } = Select;

// ClickHouse hot/cold tiering pickers. Rendered inside the create-log-library
// form, so the form instance comes from context rather than props.
const StoragePolicyFields = () => {
  const i18n = useIntl();
  const form = Form.useFormInstance();
  const { addLogToDatabase, doGetStoragePolicies } = useModel("dataLogs");
  const [policies, setPolicies] = useState<StoragePolicyResponse[]>([]);
  const isClickHouse = addLogToDatabase?.datasourceType === "ch";

  // Only ClickHouse exposes storage_policy / TTL TO VOLUME. Fetch once per
  // database selection; on any error fall back to an empty list so the UI
  // hides the tiering section instead of blocking table creation.
  useEffect(() => {
    if (!isClickHouse || !addLogToDatabase?.iid) {
      setPolicies([]);
      return;
    }
    doGetStoragePolicies
      .run(addLogToDatabase.iid)
      .then((res: any) => {
        if (res?.code === 0 && Array.isArray(res.data)) {
          setPolicies(res.data);
        } else {
          setPolicies([]);
        }
      })
      .catch(() => setPolicies([]));
  }, [isClickHouse, addLogToDatabase?.iid]);

  const selectedPolicy = Form.useWatch("storagePolicy", form);
  const volumeOptions = useMemo(() => {
    if (!selectedPolicy) return [];
    const hit = policies.find((p) => p.policyName === selectedPolicy);
    return hit?.volumes || [];
  }, [policies, selectedPolicy]);

  if (!isClickHouse) return null;

  return (
    <>
      {policies.length > 0 && (
        <Form.Item
          label={i18n.formatMessage({
            id: "datasource.logLibrary.from.storagePolicy",
          })}
          name={"storagePolicy"}
          initialValue=""
          tooltip={i18n.formatMessage({
            id: "datasource.logLibrary.from.storagePolicy.tip",
          })}
        >
          <Select
            allowClear
            placeholder={i18n.formatMessage({
              id: "datasource.logLibrary.from.storagePolicy.placeholder",
            })}
            onChange={() => {
              // Reset dependent fields when the policy changes.
              form.setFieldsValue({
                coldVolume: "",
                hotDays: undefined,
              });
            }}
          >
            <Option value="">
              {i18n.formatMessage({
                id: "datasource.logLibrary.from.storagePolicy.disabled",
              })}
            </Option>
            {policies.map((p) => (
              <Option key={p.policyName} value={p.policyName}>
                {p.policyName}
              </Option>
            ))}
          </Select>
        </Form.Item>
      )}
      {!!selectedPolicy && (
        <>
          <Form.Item
            label={i18n.formatMessage({
              id: "datasource.logLibrary.from.coldVolume",
            })}
            name={"coldVolume"}
            rules={[
              {
                required: true,
                message: i18n.formatMessage({
                  id: "datasource.logLibrary.from.coldVolume.required",
                }),
              },
            ]}
          >
            <Select
              placeholder={i18n.formatMessage({
                id: "datasource.logLibrary.from.coldVolume.placeholder",
              })}
            >
              {volumeOptions.map((v) => (
                <Option key={`${selectedPolicy}-${v.index}`} value={v.name}>
                  {v.name} (index {v.index})
                </Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item
            noStyle
            shouldUpdate={(prev, next) =>
              prev.days !== next.days || prev.hotDays !== next.hotDays
            }
          >
            {({ getFieldValue }) => {
              const days = getFieldValue("days");
              return (
                <Form.Item
                  label={i18n.formatMessage({
                    id: "datasource.logLibrary.from.hotDays",
                  })}
                  name={"hotDays"}
                  rules={[
                    {
                      required: true,
                      message: i18n.formatMessage({
                        id: "datasource.logLibrary.from.hotDays.required",
                      }),
                    },
                    {
                      validator: (_, value) => {
                        if (value == null || value === "")
                          return Promise.resolve();
                        const n = Number(value);
                        if (!Number.isInteger(n) || n <= 0)
                          return Promise.reject(
                            new Error(
                              i18n.formatMessage({
                                id: "datasource.logLibrary.from.hotDays.positive",
                              })
                            )
                          );
                        if (days != null && n >= Number(days))
                          return Promise.reject(
                            new Error(
                              i18n.formatMessage(
                                {
                                  id: "datasource.logLibrary.from.hotDays.lessThan",
                                },
                                { days }
                              )
                            )
                          );
                        return Promise.resolve();
                      },
                    },
                  ]}
                >
                  <InputNumber
                    min={1}
                    style={{ width: "100%" }}
                    placeholder={i18n.formatMessage({
                      id: "datasource.logLibrary.from.hotDays.placeholder",
                    })}
                  />
                </Form.Item>
              );
            }}
          </Form.Item>
        </>
      )}
    </>
  );
};

export default StoragePolicyFields;
