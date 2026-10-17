import { Button, Popover } from "antd";
import { useEffect, useState } from "react";
import classNames from "classnames";
import logItemStyles from "@/pages/DataLogs/components/QueryResult/Content/RawLog/RawLogList/LogItem/index.less";
import { LOGMAXTEXTLENGTH } from "@/config/config";
import { useIntl } from "umi";
import { PRE_SYMBOL } from "@/components/JsonView/JsonStringValue";
import ClickMenu from "@/pages/DataLogs/components/QueryResult/Content/RawLog/ClickMenu";
import moment from "moment";

interface onInsertQuery {
  onInsertQuery: any;
  content: any;
  keyItem: string;
  isIndexAndRawLogKey: any;
  highlightFlag: any;
  isNotTimeKey: any;
  onInsertExclusion: any;
  /** 默认展开正文（不换行模式） */
  defaultExpanded?: boolean;
}

const LogItemDetailsContent = (props: onInsertQuery) => {
  const { defaultExpanded } = props;
  const [isHidden, setIsHidden] = useState<boolean>(!defaultExpanded);
  // 切换不换行开关时，重置该条日志的展开/收起默认态
  useEffect(() => {
    setIsHidden(!defaultExpanded);
  }, [defaultExpanded]);
  const {
    onInsertQuery,
    content,
    keyItem,
    isIndexAndRawLogKey,
    onInsertExclusion,
    highlightFlag,
    isNotTimeKey,
  } = props;

  const i18n = useIntl();
  const value = isHidden
    ? content && content.length > LOGMAXTEXTLENGTH
      ? content && content.substring(0, LOGMAXTEXTLENGTH) + "..."
      : content
    : content;

  const isNewLine = (value: any) => {
    let flag = false;
    PRE_SYMBOL?.map((item: any) => {
      if (typeof value == "string" && value.indexOf(item) > 0) {
        flag = true;
      }
    });
    return flag;
  };

  return (
    <>
      {content?.length > LOGMAXTEXTLENGTH && (
        <Button
          type="primary"
          style={{
            height: "14px",
            alignItems: "center",
            display: "inline-flex",
            marginRight: "5px",
            fontSize: "12px",
          }}
          shape="round"
          size="small"
          onClick={() => setIsHidden(!isHidden)}
        >
          {isHidden
            ? i18n.formatMessage({ id: "systemSetting.role.collapseX.unfold" })
            : i18n.formatMessage({ id: "systemSetting.role.collapseX.packUp" })}
        </Button>
      )}
      {isNewLine(content) ? (
        <pre
          onClick={(e) => {
            e.stopPropagation();
          }}
          className={classNames(
            logItemStyles.logContent,
            highlightFlag && logItemStyles.logContentHighlight,
            logItemStyles.logHover
          )}
        >
          <ClickMenu
            field={keyItem}
            content={content}
            handleAddCondition={() =>
              onInsertQuery(keyItem, isIndexAndRawLogKey)
            }
            handleOutCondition={() =>
              onInsertExclusion(keyItem, isIndexAndRawLogKey)
            }
            isHidden={!isNotTimeKey}
          >
            <span>{value}</span>
          </ClickMenu>
        </pre>
      ) : (
        <span
          onClick={(e) => {
            e.stopPropagation();
          }}
          className={classNames(
            logItemStyles.logContent,
            highlightFlag && logItemStyles.logContentHighlight,
            logItemStyles.logHover
          )}
        >
          <ClickMenu
            field={keyItem}
            content={content}
            handleAddCondition={() =>
              onInsertQuery(keyItem, isIndexAndRawLogKey)
            }
            handleOutCondition={() =>
              onInsertExclusion(keyItem, isIndexAndRawLogKey)
            }
            isHidden={!isNotTimeKey}
          >
            {["ts", "time"].includes(keyItem as string) ? (
              <Popover
                content={moment(value, "X").format("YYYY-MM-DD HH:mm:ss")}
                trigger="hover"
              >
                <span>{value}</span>
              </Popover>
            ) : (
              <span>{value}</span>
            )}
          </ClickMenu>
        </span>
      )}
    </>
  );
};
export default LogItemDetailsContent;
