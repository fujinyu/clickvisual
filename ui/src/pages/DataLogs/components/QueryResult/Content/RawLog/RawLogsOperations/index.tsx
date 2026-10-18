import { FIRST_PAGE } from "@/config/config";
import { PaneType } from "@/models/datalogs/types";
import rawLogsOperationsStyles from "@/pages/DataLogs/components/QueryResult/Content/RawLog/RawLogsOperations/index.less";
import HistogramSwitch from "@/pages/DataLogs/components/QueryResult/Content/RawLog/RawLogsOperations/SwitchLeft";
import { useModel } from "@umijs/max";
import { DownloadOutlined } from "@ant-design/icons";
import { Button, message, Pagination, Popover } from "antd";
import { useMemo, useState } from "react";
import { useIntl } from "umi";

const RawLogsOperations = ({ oldPane }: { oldPane: PaneType | undefined }) => {
  const {
    logCount,
    pageSize,
    currentPage,
    onChangeLogsPage,
    currentLogLibrary,
    doGetLogsAndHighCharts,
    onChangeLogPane,
    logPanesHelper,
    resetLogPaneLogsAndHighCharts,
    downloadLogs,
  } = useModel("dataLogs");
  const { logPanes } = logPanesHelper;

  const i18n = useIntl();

  const [downloading, setDownloading] = useState<boolean>(false);

  const handleDownload = async (format: "json" | "raw") => {
    if (downloading) return;
    setDownloading(true);
    try {
      const result = await downloadLogs(format);
      if (result.count === 0) {
        message.warning(i18n.formatMessage({ id: "log.download.empty" }));
      } else if (result.truncated) {
        message.warning(
          i18n.formatMessage(
            { id: "log.download.truncated" },
            { count: result.count }
          )
        );
      } else {
        message.success(
          i18n.formatMessage(
            { id: "log.download.success" },
            { count: result.count }
          )
        );
      }
    } catch (error) {
      message.error(
        error instanceof Error && error.message
          ? error.message
          : i18n.formatMessage({ id: "log.download.failed" })
      );
    } finally {
      setDownloading(false);
    }
  };

  const performTime = useMemo(() => {
    return logPanes[currentLogLibrary?.id || 0]?.logs?.cost;
  }, [logPanes]);

  var count = useMemo(() => {
    return logPanes[currentLogLibrary?.id || 0]?.logs?.count;
  }, [logPanes]);

  if(count==0)
  {
    count=logCount;
  }

  const isLink =true

  return (
    <div className={rawLogsOperationsStyles.rawLogsOperationsMain}>
      <div className={rawLogsOperationsStyles.operationsBtn}>
        <HistogramSwitch oldPane={oldPane} />
        <Popover
          trigger="click"
          placement="bottomLeft"
          content={
            <div className={rawLogsOperationsStyles.downloadMenu}>
              <Button
                block
                size="small"
                type="text"
                disabled={downloading}
                onClick={() => handleDownload("json")}
              >
                {i18n.formatMessage({ id: "log.download.json" })}
              </Button>
              <Button
                block
                size="small"
                type="text"
                disabled={downloading}
                onClick={() => handleDownload("raw")}
              >
                {i18n.formatMessage({ id: "log.download.raw" })}
              </Button>
            </div>
          }
        >
          <Button
            size="small"
            type="text"
            icon={<DownloadOutlined />}
            loading={downloading}
          />
        </Popover>
      </div>
      {performTime ? (
        <div className={rawLogsOperationsStyles.duration}>
          {i18n.formatMessage({ id: "log.perform.time" })}:&nbsp;
          {performTime}ms
        </div>
      ) : null}
      <div className={rawLogsOperationsStyles.pagination}>
        <Pagination
          size={"small"}
          total={count}
          pageSize={pageSize}
          current={currentPage}
          pageSizeOptions={isLink ? [20, 50, 100, 200, 300] : undefined}
          showTotal={(total) => {
            return i18n.formatMessage(
              { id: "log.pagination.total" },
              { total }
            );
          }}
          onChange={(current: number, size: number) => {
            onChangeLogsPage(current, size);
            const params = {
              page: size === pageSize ? current : FIRST_PAGE,
              pageSize: size,
            };
            doGetLogsAndHighCharts(currentLogLibrary?.id as number, {
              isPaging: true,
              reqParams: params,
            })
              .then((res) => {
                if (!res) {
                  resetLogPaneLogsAndHighCharts({
                    ...(oldPane as PaneType),
                    page: size === pageSize ? current : FIRST_PAGE,
                    pageSize: size,
                  });
                } else {
                  const pane: PaneType = {
                    ...(oldPane as PaneType),
                    page: size === pageSize ? current : FIRST_PAGE,
                    pageSize: size,
                    logs: res.logs,
                    highCharts: res.highCharts,
                    logChart: { logs: [], isNeedSort: false, sortRule: ["*"] },
                  };
                  onChangeLogPane(pane);
                }
              })
              .catch(() =>
                resetLogPaneLogsAndHighCharts({
                  ...(oldPane as PaneType),
                  page: size === pageSize ? current : FIRST_PAGE,
                  pageSize: size,
                })
              );
          }}
          showSizeChanger
        />
      </div>
    </div>
  );
};
export default RawLogsOperations;
