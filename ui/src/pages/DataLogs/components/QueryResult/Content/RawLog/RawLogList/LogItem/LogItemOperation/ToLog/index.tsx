import IconFont from "@/components/IconFont";
import logItemStyles from "@/pages/DataLogs/components/QueryResult/Content/RawLog/RawLogList/LogItem/index.less";
import { useModel } from "@umijs/max";
import { Tooltip } from "antd";
import { useIntl } from "umi";


interface ToLogProps {
  log: any;
}
const ToLog = ({ log }: ToLogProps) => {
  const {
    currentLogLibrary,
    onCopyRawLogDetails,
  } = useModel("dataLogs");
  var curDate=new Date(log._time_second_)
  const i18n = useIntl();
  console.info(JSON.stringify(log))
  console.info(JSON.stringify(currentLogLibrary))
  console.info(  curDate.getTime())
  console.info(  curDate.getTime()/1000)


  const url = `${window.location.href.split("query")[0]}query?by=asc&end=${new Date(log._time_nanosecond_).getTime()/1000+60*5}&logState=0&page=1&queryType=rawLog&size=100&start=${new Date(log._time_nanosecond_).getTime()/1000}&tab=custom&tid=${currentLogLibrary?.id}`;

  return (
    <div
      className={logItemStyles.icon}
    >
      <Tooltip
          title={i18n.formatMessage({id: "log.item.to"})}
          overlayInnerStyle={{fontSize: 12}}
      > <a href={url} target="_blank">
          <IconFont type={"icon-right"}/>
        </a>
      </Tooltip>
    </div>
  );
};
export default ToLog;
