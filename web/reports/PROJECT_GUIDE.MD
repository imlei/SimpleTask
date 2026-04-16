# SimpleTask 项目指南

**SimpleTask** 是一个以**任务管理 (Task Management)** 为核心、**加拿大合规 Payroll** 为关键差异化的轻量级系统。  
系统支持将已审批的任务自动转换为符合 CRA（Canada Revenue Agency）要求的工资计算与扣缴，适用于加拿大小型企业、团队或需要简单 Payroll 合规的用户。

**部署形式**：单二进制可执行文件 + 内嵌 Web 服务器，适合本地服务器或 VPS 一键部署。

---

## 1. 核心合规模型（基于 CRA T4001）

所有 Payroll 计算必须严格**基于支付日期 (Pay Date)**，而非任务完成日期。

- **Employee (T4 路径)**：强制预扣 CPP（含 CPP2）、EI 以及联邦 + 省份所得税。
- **Contractor (T4A 路径)**：通常不预扣税款，但系统需记录其 **Business Number (BN)**，并支持 GST/HST 处理（可选）。
- **多省份支持**：员工可设置工作省份（影响 TD1、税率和税表），优先支持 BC、ON、AB 等常见省份。

---

## 2. 扣缴算法模块（Payroll Calculation Engine）

建议将计算逻辑独立为 `internal/payroll/calculator/` 包，便于单元测试和维护。

### 2.1 CPP (Canada Pension Plan)
- **年度基本豁免 (YBE)**：$3,500
- **基本公式**：
  ```text
  Period_Exemption = 3500 / Pay_Periods_Per_Year
  CPP_Base = max(0, Period_Gross - Period_Exemption) × CPP_Rate
  CPP2（第二阶段）：当 Year-to-Date (YTD) 收入超过 YMPE 后，对超过部分按 4% 收取，直至达到 YAMPE 上限。
YTD 检查：每次计算需查询员工当年累计 Gross Pay，确保不超过年度上限。

2.2 EI (Employment Insurance)

员工扣缴：min(Period_Gross, Period_Max_Insurable) × EI_Employee_Rate
雇主贡献：员工扣缴金额的 1.4 倍
达到年度最大可保收入后停止扣缴。

2.3 所得税 (Income Tax)

支持员工填写 TD1 联邦 + 省份 免税额 / Claim Codes。
推荐实现方式（优先级）：
严格遵循 CRA 官方 T4127 Payroll Deductions Formulas（2026 年最新版）
集成 CRA Payroll Deductions Online Calculator（PDOC）逻辑
短期可使用简化税表（T4032），但必须逐步升级到公式计算


计算精度要求：

使用 shopspring/decimal 或等价高精度 Decimal 类型
所有中间计算保留 4 位小数，最终四舍五入到分（0.01）


3. Task → Payroll 转换逻辑（核心差异化）

任务审批：任务状态变为 “Approved” 后自动进入 待支付池 (Payable Pool)。
周期截断：根据用户设置的发薪周期（Bi-weekly / Semi-monthly / Monthly 等），自动抓取对应 Pay Date 范围内的已审批任务。
性质判断：
Employee：任务总额计入 Gross Pay，进入完整扣缴计算引擎。
Contractor：生成包含 GST/HST（若适用）的支出明细，不进行税款预扣。

支持混合支付：同一发薪周期可同时处理 Employee 和 Contractor。

核心方法建议：
Gopayroll.RunPayroll(periodStart, periodEnd, payDate time.Time) (*PayrollResult, error)

4. 关键数据快照与审计轨迹（CRA 审计准备）
为满足 CRA 6 年记录保留要求，每次生成 Payroll 时必须创建不可变快照：

Calculation Log：记录当时使用的税率、公式版本、YMPE/YAMPE、每步计算过程
TD1 Snapshot：记录员工当时的联邦/省份免税额设置
Task Reference：每笔工资金额必须能追溯到具体的 Task ID（支持多任务合并）
存储方式：新增 payroll_snapshots 表，使用 JSON 字段存储完整计算上下文


5. 必须生成的合规文档
文档类型核心要求生成频率技术建议PaystubGross, CPP, EI, Income Tax, Net + Task 明细 + 雇主 BN每次发薪HTML 模板 + PDF 生成PD7A Summary当月所有员工扣缴总额 + 雇主贡献部分每月CSV + PDF 导出T4 / T4A符合 CRA 电子申报规范的 XML 格式每年一次XML 生成器（严格遵循 CRA SPEC）ROE符合 Service Canada 规范的离职证明 XML员工离职时XML 生成器


































文档类型核心要求生成频率技术建议PaystubGross, CPP, EI, Income Tax, Net + Task 明细 + 雇主 BN每次发薪HTML 模板 + PDF 生成PD7A Summary当月所有员工扣缴总额 + 雇主贡献部分每月CSV + PDF 导出T4 / T4A符合 CRA 电子申报规范的 XML 格式每年一次XML 生成器（严格遵循 CRA SPEC）ROE符合 Service Canada 规范的离职证明 XML员工离职时XML 生成器

## 6. 技术架构
### 6.1 核心技术栈

- 语言：Go 1.23+
- 数据库：SQLite（推荐使用 modernc.org/sqlite 纯 Go 驱动，无 CGO 依赖）
- 金额计算：shopspring/decimal
- 数据库迁移：golang-migrate/migrate
- 配置管理：spf13/viper
- PDF 生成：github.com/go-pdf/fpdf 或 github.com/jung-kurt/gofpdf
- 验证：github.com/go-playground/validator

- 推荐项目结构
cmd/simpletask/              # 主程序入口
internal/
  ├── admin/                 # 独立的管理页面 (用户管理，系统设置)
  ├── domain/                # 业务实体 (Task, Employee, PayrollPeriod 等)
  ├── payroll/
  │    ├── calculator/       # 纯计算逻辑 (CPP, EI, Tax)
  │    ├── service/          # Payroll 服务层 (生成、快照、文档)
  │    └── repository/       # Payroll 数据库操作
  ├── Invoicing/             # Invoicing的生成和管理模块
  ├── writecheque/           # 写支票的功能
  ├── task/                  # 任务管理模块
  ├── employee/              # 员工与 Contractor 管理 + TD1
  ├── config/                # tax_rates_2026.json 等配置
  ├── audit/                 # 审计与快照
  └── web/                   # HTTP handlers、模板、API
config/                      # 配置文件目录
data/                        # SQLite 数据库 + 备份文件
- 其他技术优化点

  - 安全性：SIN（Social Insurance Number）使用 AES-256-GCM 加密存储，密钥从环境变量加载。
  - 年度税率更新：所有税率参数（YMPE、YAMPE、EI 费率、税阶等）存放在 config/tax_rates_2026.json，提供 Web 界面支持“一键加载新年度配置”。
  - 备份机制：每次生成 Payroll 后自动备份数据库与快照。
  - 测试：calculator 包必须有高覆盖率单元测试，覆盖多种 YTD 场景和边界条件。
  - 可扩展性：预留未来添加 REST API、多租户（tenant_id）的空间。

## 7. 前端技术方案（现代美观 + 功能完整 + 低版权风险）
采用以下最佳平衡方案，既保持单二进制部署优势，又能快速做出现代、专业、美观的界面：

基础：Tailwind CSS v4
主题 & 基础组件：DaisyUI（提供语义化组件和多个漂亮主题，让 UI 快速变现代美观）
额外组件：HyperUI（高质量 copy-paste 组件，需要特殊卡片、表格或布局时直接使用）
整体布局参考：TailAdmin Free（MIT 许可，拿其侧边栏 + 仪表盘结构作为起点）

优势：

完全兼容 HTMX + Go html/template 或 Templ（推荐使用 Templ，提升类型安全）
支持暗黑模式、响应式设计、打印优化（Paystub 页面特别友好）
所有组件均为 MIT License，可自由用于商业项目，无版权风险

推荐集成方式：

使用 Tailwind CLI 构建 CSS（开发时 watch 模式）
将 DaisyUI 作为 Tailwind 插件加入
侧边栏、仪表盘布局参考 TailAdmin Free
具体组件（如任务表格、Payroll 预览卡片）优先使用 DaisyUI + HyperUI

适用页面示例：

仪表盘：DaisyUI Stats + TailAdmin 概览布局
任务列表 / 审批：DaisyUI Table + HTMX 局部刷新
Payroll 生成与 Paystub 预览：DaisyUI Steps + Card + 打印专用 CSS


Disclaimer（重要声明）
本系统仅供技术参考实现，不能替代专业会计师或 Payroll 软件的法律意见。
用户必须自行验证所有计算结果符合最新 CRA T4001、T4127 及相关法规要求。
建议在正式使用前由持牌 CPA 或 Payroll 专家进行审计。
税率、规则每年都会变化，系统需支持便捷的年度更新。

实施优先级建议：

高优先级：完善 Payroll Calculator（CPP/EI/Tax + YTD 处理） + 数据快照机制
中优先级：Task → Payroll 全流程 + Paystub PDF 生成
后续：T4/T4A XML 生成、多省份完整支持、自动备份与配置界面


欢迎贡献代码或提出改进建议！
如需进一步细化某个模块（例如 CPP 计算的完整 Go 代码示例、数据库表结构、PDF 模板等），请随时告诉我。