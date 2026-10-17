import { useEffect, useState } from "react";
import {
  ACTIVE_TIME_INDEX,
  FIFTEEN_TIME,
  FIRST_PAGE,
  MINUTES_UNIT_TIME,
  LOG_PAGE_SIZE,
  TimeRangeType,
} from "@/config/config";
import { currentTimeStamp } from "@/utils/momentUtils";
import moment from "moment";
import { PaneType } from "@/models/datalogs/types";
import useLocalStorages from "@/hooks/useLocalStorages";

export const DefaultPane = {
  start: moment().subtract(FIFTEEN_TIME, MINUTES_UNIT_TIME).unix(),
  end: currentTimeStamp(),
  page: FIRST_PAGE,
  pageSize: LOG_PAGE_SIZE,
  keyword: undefined,
  activeIndex: ACTIVE_TIME_INDEX,
  activeTabKey: TimeRangeType.Relative,
  highCharts: undefined,
  logs: undefined,
  histogramChecked: false,
  foldingChecked: false,
  ascChecked: false,
  rawLogChecked: true,
  fieldPanelChecked: false,
  nowrapChecked: false,
  baseFieldsIndexList: undefined,
  logFieldsIndexList: undefined,
  logState: 0,
  by: 'desc',
  relTraceTableId: 0,
  columsList: [],
};

const useLogPanes = () => {
  // 日志 Tab 标签
  const [logPanes, setLogPanes] = useState<{ [Key: string]: PaneType }>({});
  const [paneKeys, setPaneKeys] = useState<string[]>([]);
  const { onChangePaneLogPrefs, getPaneLogPrefs } = useLocalStorages();

  const removeLogPane = (key: string) => {
    const currentPanes = { ...logPanes };
    delete currentPanes[key];
    setLogPanes(currentPanes);
    setPaneKeys((paneKeys) => paneKeys.filter((item) => item !== key));
  };

  const addLogPane = (key: string, pane: PaneType) => {
    if (!paneKeys.includes(pane.paneId)) {
      setPaneKeys((paneKeys) => [...paneKeys, key]);
    }
    const panes = { ...logPanes };
    const prefs = getPaneLogPrefs(pane.paneId);
    if (prefs) {
      pane.foldingChecked = prefs.flag;
      pane.histogramChecked = prefs.histogramChecked ?? pane.histogramChecked;
      pane.rawLogChecked = prefs.rawLogChecked ?? pane.rawLogChecked;
      pane.fieldPanelChecked =
        prefs.fieldPanelChecked ?? pane.fieldPanelChecked;
      pane.nowrapChecked = prefs.nowrapChecked ?? pane.nowrapChecked;
      // URL 显式带 asc 时以 URL 为准，否则回落到该日志库记住的排序
      if (pane.by !== "asc") {
        pane.by = (prefs.ascChecked ?? pane.ascChecked) ? "asc" : "desc";
      }
      pane.ascChecked = pane.by === "asc";
    } else {
      onChangePaneLogPrefs(pane);
    }
    panes[key] = { ...DefaultPane, ...pane };
    setLogPanes(panes);
  };

  const updateLogPane = (
    key: string,
    newPane: PaneType,
    oldPanes?: { [Key: string]: PaneType }
  ) => {
    const panes = { ...(oldPanes ?? logPanes) };
    panes[key] = newPane;
    onChangePaneLogPrefs(newPane);
    setLogPanes(panes);
  };

  const resetPane = () => {
    setLogPanes({});
    setPaneKeys([]);
  };

  useEffect(() => {}, [logPanes]);

  return {
    logPanes,
    paneKeys,
    addLogPane,
    removeLogPane,
    updateLogPane,
    resetPane,
  };
};
export default useLogPanes;
