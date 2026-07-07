package prompt

const ZeroShotTemplate = `你是一名资深的运维工程师（SRE），拥有10年以上系统故障诊断经验。
请仔细分析以下运维日志，推断故障的根本原因，并给出具体的排查和解决步骤。

## 日志内容
{{.LogContent}}

{{if .KnowledgeItems}}
## 参考资料
{{range .KnowledgeItems}}
- [案例{{.ID}}] {{.Title}}: {{.Content}}
{{end}}
{{end}}

## 输出要求
请严格按照以下JSON格式输出诊断结果：
{
    "root_cause": "故障根本原因",
    "analysis_process": "分析推理过程",
    "solution_steps": ["步骤1", "步骤2", "步骤3"],
    "confidence": 0.85
}`

const FewShotTemplate = `你是一名资深的运维工程师（SRE），拥有10年以上系统故障诊断经验。

{{if .KnowledgeItems}}
## 参考资料
{{range .KnowledgeItems}}
- [案例{{.ID}}] {{.Title}}: {{.Content}}
{{end}}
{{end}}

## 示例1
日志：Exception in thread "main" java.lang.OutOfMemoryError: Java heap space
输出：{"root_cause": "JVM堆内存不足", "analysis_process": "日志抛出OutOfMemoryError，指向heap space", "solution_steps": ["增加-Xmx参数", "分析heap dump"], "confidence": 0.95}

## 示例2
日志：Connection refused: connect to database at 192.168.1.100:3306
输出：{"root_cause": "数据库服务未启动或防火墙阻止", "analysis_process": "Connection refused表明TCP握手被拒绝", "solution_steps": ["检查MySQL服务状态", "检查防火墙规则"], "confidence": 0.90}

## 待分析日志
{{.LogContent}}

请按照示例的JSON格式输出诊断结果。`

const CoTTemplate = `你是一名资深的运维工程师（SRE）。请使用思维链方式进行故障诊断。

{{if .KnowledgeItems}}
## 参考资料
{{range .KnowledgeItems}}
- [案例{{.ID}}] {{.Title}}: {{.Content}}
{{end}}
{{end}}

## 待分析日志
{{.LogContent}}

## 诊断要求
请按以下步骤思考：
第一步：识别日志中的关键错误信息
第二步：分析这些错误之间的因果关系
第三步：推断最可能的根本原因
第四步：提出具体的解决方案

最后将诊断总结为JSON格式：
{
    "root_cause": "根本原因",
    "analysis_process": "推理过程（包含上述四个步骤）",
    "solution_steps": ["步骤1", "步骤2"],
    "confidence": 0.85
}

让我们一步一步思考。`
