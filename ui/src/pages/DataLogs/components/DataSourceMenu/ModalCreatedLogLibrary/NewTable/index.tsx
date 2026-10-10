import { Form, Input, InputNumber, Select } from "antd";
import { useEffect, useMemo, useState } from "react";
import { useIntl, useModel } from "umi";
import TextArea from "antd/lib/input/TextArea";
import JsonAsString from "./JsonAsString";
import JsonEachRow from "./JsonEachRow";
import type { StoragePolicyResponse } from "@/services/dataLogs";

const { Option } = Select;

const NewTable = (props: {
  onConversionMappingJson: (str: string) => void;
  formRef: any;
  mode: number;
}) => {
  const { onConversionMappingJson, formRef, mode } = props;
  const i18n = useIntl();
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

  const selectedPolicy = Form.useWatch("storagePolicy", formRef?.current);
  const volumeOptions = useMemo(() => {
    if (!selectedPolicy) return [];
    const hit = policies.find((p) => p.policyName === selectedPolicy);
    return hit?.volumes || [];
  }, [policies, selectedPolicy]);

  return (
    <>
      <Form.Item
        label={i18n.formatMessage({
          id: "datasource.logLibrary.from.tableName",
        })}
        name={"tableName"}
        rules={[
          {
            required: true,
            message: i18n.formatMessage({
              id: "datasource.logLibrary.placeholder.tableName",
            }),
          },
          {
            pattern: new RegExp(/^[a-zA-Z_0-9]+$/),
            message: i18n.formatMessage({
              id: "datasource.logLibrary.from.rule.tableName",
            }),
          },
        ]}
      >
        <Input
          placeholder={`${i18n.formatMessage({
            id: "datasource.logLibrary.placeholder.tableName",
          })}`}
        />
      </Form.Item>

      {/* flag ? V3 : V2 */}
      {mode == 2 ? (
        <JsonAsString />
      ) : (
        <JsonEachRow
          formRef={formRef}
          onConversionMappingJson={onConversionMappingJson}
        />
      )}

      <Form.Item
        label={i18n.formatMessage({ id: "datasource.logLibrary.from.days" })}
        name={"days"}
        rules={[
          {
            required: true,
            message: i18n.formatMessage({
              id: "datasource.logLibrary.placeholder.days",
            }),
          },
        ]}
        initialValue={7}
      >
        <InputNumber
          placeholder={`${i18n.formatMessage({
            id: "datasource.logLibrary.placeholder.days",
          })}`}
          min={0}
          style={{ width: "100%" }}
        />
      </Form.Item>

      {/* ClickHouse hot/cold tiering: only shown when the target instance is CH. */}
      {isClickHouse && policies.length > 0 && (
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
              formRef?.current?.setFieldsValue({
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
      {isClickHouse && !!selectedPolicy && (
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
                        if (value == null || value === "") return Promise.resolve();
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

      <Form.Item
        label={i18n.formatMessage({
          id: "datasource.logLibrary.from.brokers",
        })}
        name={"brokers"}
        rules={[
          {
            required: true,
            message: i18n.formatMessage({
              id: "datasource.logLibrary.placeholder.brokers",
            }),
          },
        ]}
        initialValue={"172.16.3.51:9094"}
      >
        <Input
          placeholder={`${i18n.formatMessage({
            id: "datasource.logLibrary.placeholder.brokers",
          })}`}
        />
      </Form.Item>
      <Form.Item
        label={i18n.formatMessage({
          id: "datasource.logLibrary.from.topics",
        })}
        name={"topics"}
        rules={[
          {
            required: true,
            message: i18n.formatMessage({
              id: "datasource.logLibrary.placeholder.topics",
            }),
          },
          {
            pattern: new RegExp(/^[a-zA-Z0-9\-_.]+$/),
            message: i18n.formatMessage({
              id: "datasource.logLibrary.from.rule.topics",
            }),
          },
        ]}
      >
        <Input
          placeholder={`${i18n.formatMessage({
            id: "datasource.logLibrary.placeholder.topics",
          })}`}
        />
      </Form.Item>
      <Form.Item
        label={i18n.formatMessage({
          id: "datasource.logLibrary.from.consumers",
        })}
        name={"consumers"}
        rules={[
          {
            required: true,
            message: i18n.formatMessage({
              id: "datasource.logLibrary.placeholder.consumers",
            }),
          },
        ]}
        initialValue={1}
      >
        <InputNumber
          min={0}
          style={{ width: "100%" }}
          placeholder={`${i18n.formatMessage({
            id: "datasource.logLibrary.placeholder.consumers",
          })}`}
        />
      </Form.Item>
      <Form.Item
        label={"kafkaSkipBrokenMessages"}
        name={"kafkaSkipBrokenMessages"}
        initialValue={10}
      >
        <InputNumber min={0} style={{ width: "100%" }} />
      </Form.Item>

      <Form.Item
        label={i18n.formatMessage({
          id: "description",
        })}
        name="desc"
      >
        <TextArea
          rows={3}
          placeholder={i18n.formatMessage({
            id: "datasource.logLibrary.from.newLogLibrary.desc.placeholder",
          })}
        ></TextArea>
      </Form.Item>
    </>
  );
};
export default NewTable;
