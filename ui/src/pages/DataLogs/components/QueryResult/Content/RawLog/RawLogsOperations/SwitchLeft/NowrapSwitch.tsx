import { PaneType } from "@/models/datalogs/types";
import switchStyles from "@/pages/DataLogs/components/QueryResult/Content/RawLog/RawLogsOperations/SwitchLeft/index.less";
import { useModel } from "@umijs/max";
import { Switch } from "antd";
import { useIntl } from "umi";

// 不换行开关：打开后日志正文单行展示，超长部分在行内横向滚动
const NowrapSwitch = ({ oldPane }: { oldPane: PaneType | undefined }) => {
  const { logPanesHelper } = useModel("dataLogs");
  const { updateLogPane, logPanes } = logPanesHelper;

  const handleChangeNowrapChecked = () => {
    if (!oldPane) return;
    updateLogPane(
      oldPane.paneId,
      { ...oldPane, nowrapChecked: !oldPane?.nowrapChecked },
      logPanes
    );
  };

  const i18n = useIntl();
  return (
    <>
      <Switch
        checked={oldPane?.nowrapChecked ?? false}
        onChange={handleChangeNowrapChecked}
        size={"small"}
      />
      <span className={switchStyles.title} onClick={handleChangeNowrapChecked}>
        {i18n.formatMessage({ id: "log.switch.nowrap" })}
      </span>
    </>
  );
};
export default NowrapSwitch;
