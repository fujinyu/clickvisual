import { PaneType } from "@/models/datalogs/types";
import switchStyles from "@/pages/DataLogs/components/QueryResult/Content/RawLog/RawLogsOperations/SwitchLeft/index.less";
import { useModel } from "@umijs/max";
import { Switch } from "antd";
import { useIntl } from "umi";

const HistogramSwitch = ({ oldPane }: { oldPane: PaneType | undefined }) => {
  const { logPanesHelper, doGetLogs} = useModel("dataLogs");
  const { updateLogPane, logPanes } = logPanesHelper;

  const handleChangeHistogramChecked = async () => {
    if (!oldPane) return;
    if (!oldPane.ascChecked) {
       await doGetLogs({
           by: "asc"
       });
    } else {
        await doGetLogs({
            by: "desc"
        });
    }
    updateLogPane(
      oldPane.paneId,
      { ...oldPane, ascChecked: !oldPane?.ascChecked },
      logPanes
    );
  };

  const i18n = useIntl();
  return (
    <>
      <Switch
        checked={oldPane?.ascChecked ?? true}
        onChange={handleChangeHistogramChecked}
        size={"small"}
      />
      <span
        className={switchStyles.title}
        onClick={handleChangeHistogramChecked}
      >
        {i18n.formatMessage({ id: "log.switch.asc" })}
      </span>
    </>
  );
};
export default HistogramSwitch;
