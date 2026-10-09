import { PaneType } from "@/models/datalogs/types";
import switchStyles from "@/pages/DataLogs/components/QueryResult/Content/RawLog/RawLogsOperations/SwitchLeft/index.less";
import { useModel } from "@umijs/max";
import { Switch } from "antd";
import { useIntl } from "umi";

// 仅原始日志开关：打开后日志列表只显示 _raw_log_ 字段，便于快速查看原始报文
const RawLogSwitch = ({ oldPane }: { oldPane: PaneType | undefined }) => {
  const { logPanesHelper } = useModel("dataLogs");
  const { updateLogPane, logPanes } = logPanesHelper;

  const handleChangeRawLogChecked = () => {
    if (!oldPane) return;
    updateLogPane(
      oldPane.paneId,
      { ...oldPane, rawLogChecked: !oldPane?.rawLogChecked },
      logPanes
    );
  };

  const i18n = useIntl();
  return (
    <>
      <Switch
        checked={oldPane?.rawLogChecked ?? false}
        onChange={handleChangeRawLogChecked}
        size={"small"}
      />
      <span className={switchStyles.title} onClick={handleChangeRawLogChecked}>
        {i18n.formatMessage({ id: "log.switch.rawLogOnly" })}
      </span>
    </>
  );
};
export default RawLogSwitch;
