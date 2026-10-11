import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import LogLibraryManagementPage from "../src/domains/logLibrary/pages/LogLibraryManagementPage";

const INSTANCE_ID = 101;
const DATABASE_ID = 8;
const TABLE_ID = 9527;

function jsonResponse(data: unknown) {
  return {
    ok: true,
    status: 200,
    text: async () => JSON.stringify({ code: 0, msg: "succ", data })
  };
}

function createFetchMock() {
  const createPayloads: Record<string, unknown>[] = [];
  const updatePayloads: Record<string, unknown>[] = [];
  const templatePayloads: Record<string, unknown>[] = [];
  const rebuildIds: number[] = [];
  const mock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(
        typeof input === "string" ? input : input.toString(),
        "http://localhost"
      );
      const method = init?.method || "GET";
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;

      if (method === "POST" && url.pathname.endsWith("/api/v1/pms/check")) {
        return jsonResponse(true);
      }
      if (method === "GET" && url.pathname.endsWith("/api/v2/base/instances")) {
        return jsonResponse([
          {
            id: INSTANCE_ID,
            instanceName: "核心日志实例",
            desc: "primary",
            databases: [
              {
                id: DATABASE_ID,
                iid: INSTANCE_ID,
                databaseName: "app_db",
                tables: [
                  { id: TABLE_ID, did: DATABASE_ID, tableName: "app_logs", desc: "" }
                ]
              }
            ]
          }
        ]);
      }
      if (
        method === "GET" &&
        url.pathname.endsWith(`/api/v2/instances/${INSTANCE_ID}/storage-policies`)
      ) {
        return jsonResponse([
          {
            policyName: "cold_policy",
            volumes: [{ name: "cold_vol", index: 1, disks: ["disk_cold"] }]
          }
        ]);
      }
      if (method === "POST" && url.pathname.endsWith("/storage/preview-json")) {
        return jsonResponse({
          mode: "standalone",
          tableCount: 1,
          tableRoles: ["local"],
          timeField: "timestamp",
          timeFieldType: 2,
          rawLogField: "_raw_log_",
          fields: [
            { key: "timestamp", path: "timestamp", type: "Float64", isTime: true },
            { key: "_raw_log_", path: "_raw_log_", type: "String", isRawLog: true }
          ],
          timeCandidates: [
            { key: "timestamp", path: "timestamp", type: "Float64", timeFieldType: 2 }
          ],
          rawLogCandidates: [{ key: "_raw_log_", path: "_raw_log_", type: "String" }]
        });
      }
      if (
        method === "POST" &&
        url.pathname.endsWith("/storage/ilogtail_k8s")
      ) {
        templatePayloads.push(body);
        return jsonResponse(null);
      }
      if (
        method === "POST" &&
        url.pathname.endsWith("/log-library-management/storage")
      ) {
        createPayloads.push(body);
        return jsonResponse(null);
      }
      if (method === "GET" && /\/api\/v1\/tables\/\d+$/.test(url.pathname)) {
        return jsonResponse({
          id: TABLE_ID,
          name: "app_logs",
          desc: "existing",
          days: 7,
          topic: "app_logs",
          brokers: "kafka:9092",
          consumerNum: 1,
          kafkaSkipBrokenMessages: 10,
          v3TableType: 0,
          storagePolicy: "cold_policy",
          coldVolume: "cold_vol",
          hotDays: 3
        });
      }
      if (method === "PATCH" && /\/api\/v2\/storage\/\d+$/.test(url.pathname)) {
        updatePayloads.push(body);
        return jsonResponse(null);
      }
      if (
        method === "POST" &&
        /\/api\/v1\/tables\/(\d+)\/rebuild$/.test(url.pathname)
      ) {
        const matched = url.pathname.match(/\/api\/v1\/tables\/(\d+)\/rebuild$/);
        rebuildIds.push(Number(matched?.[1]));
        return jsonResponse(null);
      }
      return jsonResponse(null);
    }
  );
  return { mock, createPayloads, updatePayloads, templatePayloads, rebuildIds };
}

describe("LogLibraryManagementPage hot/cold tiering", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("submits tiering fields when creating a ClickHouse log library", async () => {
    const { mock, createPayloads } = createFetchMock();
    vi.stubGlobal("fetch", mock);
    render(<LogLibraryManagementPage />);

    fireEvent.click(await screen.findByText("新增日志库"));
    fireEvent.change(screen.getByLabelText(/数据库/), {
      target: { value: String(DATABASE_ID) }
    });
    fireEvent.change(screen.getByLabelText(/JSON 日志样例/), {
      target: { value: '{"timestamp": 1710000000}' }
    });
    fireEvent.click(screen.getByText("解析并预览"));
    await screen.findByText(/已识别/);

    fireEvent.change(screen.getByLabelText(/日志表名/), {
      target: { value: "app_logs" }
    });

    const policySelect = await screen.findByLabelText(/存储策略/);
    fireEvent.change(policySelect, { target: { value: "cold_policy" } });
    const volumeSelect = await screen.findByLabelText(/冷数据 Volume/);
    fireEvent.change(volumeSelect, { target: { value: "cold_vol" } });
    fireEvent.change(screen.getByLabelText(/热数据天数/), {
      target: { value: "3" }
    });

    fireEvent.click(screen.getByText("创建日志库"));
    await waitFor(() => expect(createPayloads).toHaveLength(1));
    expect(createPayloads[0]).toMatchObject({
      storagePolicy: "cold_policy",
      coldVolume: "cold_vol",
      hotDays: 3
    });
  });

  it("updates hot days from the edit log library modal", async () => {
    const { mock, updatePayloads } = createFetchMock();
    vi.stubGlobal("fetch", mock);
    render(<LogLibraryManagementPage />);

    fireEvent.click(await screen.findByRole("button", { name: "编辑" }));
    const hotDaysInput = await screen.findByLabelText(/热数据天数/);
    await waitFor(() => expect(hotDaysInput).toHaveValue(3));

    fireEvent.change(hotDaysInput, { target: { value: "5" } });
    fireEvent.click(screen.getByText("保存"));

    await waitFor(() => expect(updatePayloads).toHaveLength(1));
    expect(updatePayloads[0]).toMatchObject({
      hotDays: 5,
      mergeTreeTTL: 7,
      kafkaTopic: "app_logs",
      kafkaBrokers: "kafka:9092"
    });
  });

  it("creates a log library via the iLogtail K8s template", async () => {
    const { mock, templatePayloads } = createFetchMock();
    vi.stubGlobal("fetch", mock);
    render(<LogLibraryManagementPage />);

    fireEvent.click(await screen.findByText("iLogtail K8s 接入"));
    fireEvent.change(await screen.findByLabelText(/数据库/), {
      target: { value: String(DATABASE_ID) }
    });
    fireEvent.change(screen.getByLabelText(/日志表名/), {
      target: { value: "k8s_logs" }
    });
    fireEvent.change(screen.getByLabelText(/Kafka Brokers/), {
      target: { value: "kafka:9092" }
    });
    fireEvent.change(screen.getByLabelText(/Kafka Topic/), {
      target: { value: "k8s_topic" }
    });
    fireEvent.change(screen.getByLabelText(/TTL 天数/), {
      target: { value: "7" }
    });
    fireEvent.click(screen.getByText("确认接入"));

    await waitFor(() => expect(templatePayloads).toHaveLength(1));
    expect(templatePayloads[0]).toMatchObject({
      databaseId: DATABASE_ID,
      name: "k8s_logs",
      brokers: "kafka:9092",
      topic: "k8s_topic",
      days: 7
    });
  });

  it("rebuilds the collection schema after confirmation", async () => {
    const { mock, rebuildIds } = createFetchMock();
    vi.stubGlobal("fetch", mock);
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<LogLibraryManagementPage />);

    fireEvent.click(await screen.findByRole("button", { name: "重建采集" }));

    await waitFor(() => expect(rebuildIds).toEqual([TABLE_ID]));
    expect(confirmSpy).toHaveBeenCalledTimes(1);
    confirmSpy.mockRestore();
  });
});
