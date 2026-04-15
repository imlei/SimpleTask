# CRA T4001 Payroll Deductions & Remittances — 核心规则摘要

> 来源：T4001(E) Rev. 25（2025 年版）  
> 原始 PDF 已由用户提供，建议每年核对最新版本。  
> **免责声明**：本文仅供技术参考，不构成税务或法律意见，实际使用请咨询 CPA。

---

## 一、2025 年关键数字速查

| 项目 | 2025 数值 |
|------|----------|
| CPP 缴费率（员工/雇主各） | **5.95%** |
| CPP 最高应缴薪资 (YMPE) | **$71,300** |
| CPP 年度基础豁免 | **$3,500** |
| CPP 员工最高年缴额 | **$4,034.10** |
| CPP2 缴费率（第二阶段） | **4%** |
| CPP2 最高应缴薪资 (YAMPE) | **$81,200** |
| EI 员工费率（非魁省） | **1.64%** |
| EI 员工费率（魁省） | **1.31%** |
| EI 最高可保收入 | **$65,700** |
| EI 员工最高年缴额（非魁省） | **$1,077.48** |
| EI 员工最高年缴额（魁省） | **$860.67** |
| EI 雇主倍数 | **1.4×** 员工缴额 |
| 魁省 QPP 缴费率 | **6.40%** |

---

## 二、CPP（Canada Pension Plan）

### 2.1 何时扣缴

满足以下**全部条件**才须扣 CPP：
- 员工处于应缴薪资雇佣状态
- 员工**不属于**CPP/QPP 残障认定
- 员工年龄 **18–69 岁**（例外：65–69 岁且已领 CPP 退休金，可填 Form CPT30 选择停止缴纳）

### 2.2 计算公式（每个发薪周期）

```
CPP 扣缴额 = (应缴薪资 - 当期豁免额) × 5.95%
```

**各发薪周期豁免额（2025）**：

| 发薪周期 | 豁免额 |
|---------|--------|
| 每年 (1) | $3,500.00 |
| 半年 (2) | $1,750.00 |
| 季度 (4) | $875.00 |
| 每月 (12) | $291.66 |
| 半月 (24) | $145.83 |
| 双周 (26) | $134.61 |
| 双周 (27) | $129.62 |
| 每周 (52) | $67.30 |
| 每周 (53) | $66.03 |

### 2.3 CPP2（第二阶段，2024 年起）

```
CPP2 = 超过 $71,300 部分（上限 $81,200）× 4%
```
- **不适用** $3,500 豁免
- 魁省使用 QPP2，不用 CPP2

### 2.4 雇主义务

雇主缴额 = 员工扣缴额（1:1 匹配）

### 2.5 跨年度/部分年份处理

需按比例计算最高缴额：
```
最高缴额 = ($71,300 - $3,500) × 应缴月数/12 × 5.95%
```

**特殊情况**（均需按比例计算）：
- 员工在年中满 18 岁 → 从满 18 岁当月的**下一个月**第一个发薪日开始扣
- 员工在年中满 70 岁 → 到满 70 岁**当月**最后发薪日止
- 员工在年中残障 → 到认定残障当月最后发薪日止
- 员工在年中去世 → 到去世当月最后发薪日止（含死亡前已欠未付）

### 2.6 不须扣 CPP 的情形

- 农业/渔业/林业，当年薪资 < $250 **且** 工作天数 < 25 天
- 非常规临时雇佣（casual，非正常业务）
- 宗教团体（已立誓贫穷）成员
- 政府机构选举工作者（< 35 小时/年）
- 配偶/同居伴侣（若薪资不能作为税务抵扣费用）

---

## 三、EI（Employment Insurance）

### 3.1 计算公式

```
员工 EI = min(可保收入, $65,700) × 1.64%  （魁省 1.31%）
雇主 EI = 员工 EI × 1.4
```

- 可保收入达到 $65,700 后停止扣缴
- 不设年龄限制

### 3.2 魁省差异

魁省雇主须同时扣缴：
- **EI**（费率 1.31%，因为 QPIP 替代了部分 EI 福利）
- **QPIP**（Quebec Parental Insurance Plan）→ 报送 Revenu Québec

### 3.3 不须扣 EI 的情形

| 情形 | 说明 |
|------|------|
| 非常规临时雇佣 | 非正常业务目的 |
| 非公平交易（non-arm's length） | 关联人员（家庭成员等），视具体情况 |
| 持超过 40% 投票股的公司股东 | 从该公司领薪 |
| 内部职务持有人 | 市长、市议员、遗产管理人等 |
| 宗教团体（立誓贫穷） | — |
| 工作/服务互换 | — |
| 农业雇佣 < 7 天/年或无现金薪酬 | — |

### 3.4 可保工时（Insurable Hours）

- **时薪制**：实际工作并获付报酬的小时数
- **非时薪制**：雇主知晓实际工时则用实际工时；不知晓时用`可保收入 ÷ 最低时薪`（不超过 7 小时/天或 35 小时/周）
- 加班、假日、带薪假：1 小时工作 = 1 小时可保工时（即使加班费率更高）
- **无工时附加报酬**（奖金、小费、代通知金）：不产生可保工时

---

## 四、所得税（Income Tax）

### 4.1 核心工具

| 工具 | 用途 |
|------|------|
| **PDOC**（canada.ca/pdoc）| 在线计算器，最常用 |
| T4032 *Payroll Deductions Tables* | 印刷表格，按发薪周期 |
| T4008 | 非常规发薪周期补充表 |
| T4127 *Payroll Deductions Formulas* | 程序化公式，适合系统集成 |

### 4.2 TD1 Personal Tax Credits Return

- 员工入职时必须提交 TD1（联邦）及省级 TD1
- 未提交 TD1 → 使用基础个人免税额（Claim Code 1）
- 员工情况变化后 7 天内须提交新 TD1
- 魁省：联邦 TD1 + 省级 Form TP-1015.3-V

### 4.3 特殊薪资的所得税处理

**奖金/追溯加薪（Bonus Method）**：
1. 将奖金除以年内发薪次数得到调整额
2. 加到正常发薪额得到调整后发薪额
3. 从调整后税额减去正常税额，得差额
4. 差额 × 年内发薪次数 = 应从奖金扣缴的税额

简便规则：
- 年收入（含奖金）≤ $5,000：扣 15%（魁省 10%）
- 年收入（含奖金）> $5,000：用正式 Bonus Method

**退休金（Retiring Allowance / Severance）** — 分级预扣税：
| 金额 | 税率（非魁省混合） | 魁省联邦 |
|------|------|------|
| ≤ $5,000 | 10% | 5% |
| $5,001–$15,000 | 20% | 10% |
| > $15,000 | 30% | 15% |

**税务减免项**（扣税前可减除）：
- RPP（Registered Pension Plan）员工缴款
- RRSP 代扣缴款
- FHSA 代扣缴款
- 工会会费
- 偏远地区居住扣除
- 税务局批准的 Letter of Authority 授权金额

**注意**：CPP 和 EI 不能用于减少应税薪资基数。

---

## 五、发薪 → 扣缴 → 汇款 完整流程

```
每个发薪周期
  1. 计算 Gross Pay
  2. 扣除可减免项（RPP、RRSP 等）→ 得 Net Taxable Remuneration
  3. 计算 CPP（按豁免额）
  4. 计算 CPP2（若超过 YMPE）
  5. 计算 EI
  6. 计算所得税（用 PDOC/T4032/T4127）
  7. 净发放金额 = Gross Pay - CPP - CPP2 - EI - Income Tax
  8. 汇款 = 员工 CPP + 雇主 CPP + 员工 CPP2 + 雇主 CPP2
             + 员工 EI + 雇主 EI + 员工所得税
```

---

## 六、汇款（Remittance）类型与截止日

| 汇款人类型 | AMWA | 截止日 |
|---------|------|--------|
| **Quarterly**（新小雇主） | MWA < $1,000 且 AMWA < $3,000 | 每季末次月 15 日（4/15、7/15、10/15、1/15） |
| **Regular** | < $25,000 | 次月 15 日 |
| **Accelerated T1** | $25,000–$99,999 | 1–15 日发薪 → 当月 25 日；16–月末发薪 → 次月 10 日 |
| **Accelerated T2** | ≥ $100,000 | 每 7 天一个周期，周期末 3 个工作日内通过金融机构汇款 |

**T2 的四个周期**：1–7日、8–14日、15–21日、22–月末

> AMWA = Average Monthly Withholding Amount，基准为**两年前**的月均扣缴总额

### 逾期罚款

| 逾期天数 | 罚款率 |
|---------|-------|
| 1–3 天 | 3% |
| 4–5 天 | 5% |
| 6–7 天 | 7% |
| > 7 天或未汇 | 10% |
| 二次违规（故意/重大疏忽） | 20% |

**> $500 的未汇款才全额计算罚款**（小额仅超出部分）  
**未按时在金融机构处理**（即到期日在非金融机构支付）：另加 3% 罚款  
**未扣缴 CPP/EI** 罚款：10%（重复故意：20%）  
**刑事处罚**：$1,000–$25,000 罚款或最高 12 个月监禁

### 汇款截止日恰逢周末/假日

若到期日为周六、周日或 CRA 认可的公共假日，**顺延至下一工作日**有效。

---

## 七、特殊支付速查表（Appendix 4 精华）

| 支付类型 | CPP | EI | 所得税 |
|---------|-----|----|--------|
| 工资/薪水 | ✓ | ✓ | ✓ |
| 奖金/加薪 | ✓ | ✓ | ✓ |
| 预支工资 | ✓ | ✓ | ✓ |
| 加班费 | ✓ | ✓ | ✓ |
| 带薪假期/病假 | ✓ | ✓ | ✓ |
| 假期工资（取假） | ✓ | ✓ | ✓ |
| 假期工资（不取假，奖金法） | ✓ | ✓ | ✓ |
| 通知代偿金 | ✓ | ✓ | ✓ |
| 遣散费/退休补偿 | ✗ | ✗ | ✓（分级税率） |
| 员工分红计划（EPSP） | ✗ | ✗ | ✗ |
| 员工死亡后支付（生前已赚） | ✓ | ✗ | ✓ |
| 工伤补偿（正式赔偿等额预支） | ✗ | ✗ | ✗ |
| 工伤补偿 Top-up（赔偿决定后）| ✓ | ✗ | ✓ |
| EI 法定福利 | ✗ | ✗ | ✓ |
| 退休补偿安排（RCA）分配 | ✗ | ✗ | ✓ |

---

## 八、雇主核心合规义务清单

1. **注册 Payroll Program Account**（Business Number + RP 账户）
2. **收集员工 SIN**（入职 3 天内）
3. **收集 TD1 表**（联邦 + 省/地区）
4. 每个发薪周期**扣缴 CPP + CPP2 + EI + 所得税**，作为信托资金单独保管
5. 按时**汇款**至 Receiver General（含雇主份额）
6. 每年 2 月底前**提交 T4/T4A 信息回报**
7. 员工离职时**开具 ROE**（Record of Employment，电子发出：5 个日历日内）
8. **保存记录 6 年**

---

## 九、魁省特殊规则汇总

| 项目 | 魁省 | 其他省份 |
|------|------|---------|
| 养老金计划 | **QPP**（6.40%） | CPP（5.95%） |
| 育儿保险 | **QPIP** | EI 覆盖 |
| EI 费率 | **1.31%** | 1.64% |
| 汇款对象 | QPP/QPIP/省税 → Revenu Québec；EI/联邦税 → CRA | 全部 → CRA |
| CPP2 | 使用 **QPP2** | CPP2 |
| 税表 | 联邦 TD1 + 省级 TP-1015.3-V | 联邦 TD1 + 省级 TD1 |
| 工会会费减免 | 省级规则不同 | 正常减免 |

---

## 十、计算精度要求（来自 PROJECT_GUIDE）

- 使用 `shopspring/decimal` 或等价高精度 Decimal 类型
- 中间计算保留 4 位小数
- 最终四舍五入到分（0.01）

---

## 十一、核心计算公式代码示例（Go 伪代码）

```go
type PayrollCalc struct {
    GrossPay       decimal.Decimal
    PayPeriods     int  // 26 = biweekly
    YTDGross       decimal.Decimal
    YTDCpp         decimal.Decimal
    YTDEI          decimal.Decimal
    Province       string
    TD1Federal     decimal.Decimal
    TD1Provincial  decimal.Decimal
}

// CPP 计算
func (p *PayrollCalc) CalcCPP() decimal.Decimal {
    // YMPE 2025
    ympe := decimal.NewFromFloat(71300)
    annualExemption := decimal.NewFromFloat(3500)
    rate := decimal.NewFromFloat(0.0595)
    maxAnnual := decimal.NewFromFloat(4034.10)

    // 当期豁免额
    periodExemption := annualExemption.Div(decimal.NewFromInt(int64(p.PayPeriods)))

    // 可缴收入
    pensionable := p.GrossPay.Sub(periodExemption)
    if pensionable.LessThan(decimal.Zero) {
        pensionable = decimal.Zero
    }

    cpp := pensionable.Mul(rate)

    // YTD 上限检查
    remaining := maxAnnual.Sub(p.YTDCpp)
    if cpp.GreaterThan(remaining) {
        cpp = remaining
    }
    return cpp.Round(2)
}

// CPP2 计算
func (p *PayrollCalc) CalcCPP2() decimal.Decimal {
    ympe := decimal.NewFromFloat(71300)
    yampe := decimal.NewFromFloat(81200)
    rate := decimal.NewFromFloat(0.04)

    ytdAfterCPP := p.YTDGross
    if ytdAfterCPP.LessThanOrEqual(ympe) {
        return decimal.Zero
    }
    // 仅对超过 YMPE 的部分按 4% 计算
    above := ytdAfterCPP.Sub(ympe)
    if above.GreaterThan(yampe.Sub(ympe)) {
        above = yampe.Sub(ympe)
    }
    return above.Mul(rate).Round(2)
}

// EI 计算
func (p *PayrollCalc) CalcEI() decimal.Decimal {
    maxInsurable := decimal.NewFromFloat(65700)
    maxPremium := decimal.NewFromFloat(1077.48)
    rate := decimal.NewFromFloat(0.0164)
    if p.Province == "QC" {
        rate = decimal.NewFromFloat(0.0131)
        maxPremium = decimal.NewFromFloat(860.67)
    }

    insurable := p.GrossPay
    // YTD 上限
    ytdRemaining := maxInsurable.Sub(p.YTDGross)
    if insurable.GreaterThan(ytdRemaining) {
        insurable = ytdRemaining
    }
    if insurable.LessThan(decimal.Zero) {
        return decimal.Zero
    }
    ei := insurable.Mul(rate)
    premiumRemaining := maxPremium.Sub(p.YTDEI)
    if ei.GreaterThan(premiumRemaining) {
        ei = premiumRemaining
    }
    return ei.Round(2)
}
```

---

## 十二、年末合规文档

| 文档 | 截止日 | 说明 |
|------|-------|------|
| T4 Slip | 次年 2 月底 | 每位员工 |
| T4 Summary | 次年 2 月底 | 汇总 |
| T4A | 次年 2 月底 | 承包商/退休金等 |
| ROE | 离职后 5 个日历日 | 电子发出 |
| PD7A | 每个汇款周期 | 汇款凭证 |

**超过 5 份 T4 必须电子申报（XML 或 Web Forms）**

---

*最后更新：2026-04-15，基于 T4001(E) Rev. 25*
