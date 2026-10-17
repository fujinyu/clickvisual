import { UrlStateType } from "@/pages/DataLogs/hooks/useLogUrlParams";

interface FoldLogStorageType {
  tid: string;
  /**
   * 折叠日志开关，沿用历史字段名，避免老数据失效
   */
  flag: boolean;
  histogramChecked?: boolean;
  ascChecked?: boolean;
  rawLogChecked?: boolean;
  fieldPanelChecked?: boolean;
}

const PANE_STORAGE_KEY = "clickvisual-fold-log-flag";

export interface PanePrefsInput {
  paneId: string;
  histogramChecked: boolean;
  foldingChecked: boolean;
  ascChecked: boolean;
  rawLogChecked: boolean;
  fieldPanelChecked?: boolean;
}

const toPaneStorage = (pane: PanePrefsInput): FoldLogStorageType => ({
  tid: pane.paneId,
  flag: pane.foldingChecked,
  histogramChecked: pane.histogramChecked,
  ascChecked: pane.ascChecked,
  rawLogChecked: pane.rawLogChecked,
  fieldPanelChecked: pane.fieldPanelChecked,
});

export enum LocalModuleType {
  dataLogs = "data-Logs",
  dataAnalysis = "data-analysis",
  datalogsQuerySql = "datalogs-query-sql",
  dataAnalysisOpenNodeId = "data-analysis-open-node-id",
  datalogsFilterDisableIds = "datalogs-filter-disable-ids",
}

export interface LastDataLogsStateType extends UrlStateType {
  querySql?: any;
  where?: string;
}

const useLocalStorages = () => {
  const getPaneLogPrefs = (tid: string) => {
    const foldLogStorages: FoldLogStorageType[] = JSON.parse(
      localStorage.getItem(PANE_STORAGE_KEY) || "[]"
    );
    return foldLogStorages.find((item: any) => item.tid === tid);
  };

  const onChangePaneLogPrefs = (pane: PanePrefsInput) => {
    const foldLogStorages: FoldLogStorageType[] = JSON.parse(
      localStorage.getItem(PANE_STORAGE_KEY) || "[]"
    );
    const currentIndex = foldLogStorages.findIndex(
      (item: any) => item.tid === pane.paneId
    );
    if (currentIndex > -1) {
      foldLogStorages[currentIndex] = {
        ...foldLogStorages[currentIndex],
        ...toPaneStorage(pane),
      };
    } else {
      foldLogStorages.push(toPaneStorage(pane));
    }
    localStorage.setItem(
      PANE_STORAGE_KEY,
      JSON.stringify(foldLogStorages)
    );
  };

  const getLastDataLogsState = () => {
    const lastDataLogsState: LastDataLogsStateType = JSON.parse(
      localStorage.getItem("clickvisual-last-datalogs-state") || "[]"
    );
    return lastDataLogsState;
  };

  const onChangeDataLogsState = (value: LastDataLogsStateType) => {
    localStorage.setItem(
      "clickvisual-last-datalogs-state",
      JSON.stringify(value)
    );
  };
  /**
   * 查取localStorage的值
   * moduleName 不存在则写入
   * moduleName 存在则更新
   * moduleName 存在且value为undefined则为读取
   * @param value
   * @param moduleName
   * @returns
   */
  const onSetLocalData = (value: any, moduleName: string) => {
    try {
      let allClickHouse = JSON.parse(
        localStorage.getItem("clickvisual") || JSON.stringify({})
      );
      let oldClickHouse = allClickHouse[moduleName] || {};
      if (value === undefined) {
        return oldClickHouse;
      }
      if (value === null) {
        allClickHouse[moduleName] = undefined;
        localStorage.setItem("clickvisual", JSON.stringify(allClickHouse));
        return true;
      }
      const newClickHouse = value;
      const newKeys = Object.keys(newClickHouse);
      const oldKeys = Object.keys(oldClickHouse);
      let returnObj = {};
      let isChange: boolean = false;
      newKeys.map((item: any) => {
        // 如果该key不存在，则直接写入local里并返回该值
        if (!oldKeys.includes(item)) {
          oldClickHouse[item] = newClickHouse[item];
          isChange = true;
          returnObj[item] = newClickHouse[item];
          return;
        }
        // 如果key存在 则对比local和传入的值的新旧关系
        if (newClickHouse[item] !== undefined) {
          // 相同则不处理
          if (newClickHouse[item] == oldClickHouse[item]) {
            returnObj[item] = newClickHouse[item];
            return;
          }
          oldClickHouse[item] = newClickHouse[item];
          isChange = true;
          returnObj[item] = newClickHouse[item];
          return;
        }
        // 当且仅当key的value为undefined时才会取local里的值当做返回值
        returnObj[item] = oldClickHouse[item];
      });
      allClickHouse[moduleName] = oldClickHouse;
      isChange &&
        localStorage.setItem("clickvisual", JSON.stringify(allClickHouse));
      return returnObj;
    } catch (e) {
      console.error("localaStorage存取的onSetLocalData函数内部执行出错");
      return false;
    }
  };

  return {
    getPaneLogPrefs,
    onChangePaneLogPrefs,
    getLastDataLogsState,
    onChangeDataLogsState,
    onSetLocalData,
  };
};
export default useLocalStorages;
