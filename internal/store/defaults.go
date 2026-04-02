package store

import "simpletask/internal/models"

func f64(v float64) *float64 { return &v }

// DefaultPriceList 根据价目表图片初始化的默认数据（可之后在界面中修改）
func DefaultPriceList() []models.PriceItem {
	return []models.PriceItem{
		{ID: "P0001", ServiceName: "BC省公司注册", Amount: f64(3500), Currency: models.CNY},
		{ID: "P0002", ServiceName: "BC省公司年审", Amount: f64(500), Currency: models.CNY},
		{ID: "P0003", ServiceName: "BC省公司董事变更", Amount: f64(65), Currency: models.USD},
		{ID: "P0004", ServiceName: "BC公司零报税", Amount: f64(200), Currency: models.CAD},
		{ID: "P0005", ServiceName: "BC省goodstanding", Amount: f64(65), Currency: models.CAD},
		{ID: "P0006", ServiceName: "安省公司注册", Amount: f64(550), Currency: models.CAD},
		{ID: "P0007", ServiceName: "安省公司年审", Amount: f64(500), Currency: models.CNY},
		{ID: "P0008", ServiceName: "联邦公司注册", Amount: f64(3500), Currency: models.CNY},
		{ID: "P0009", ServiceName: "联邦公司年审", Amount: f64(500), Currency: models.CNY},
		{ID: "P0010", ServiceName: "亚伯达省公司注册", Amount: f64(1000), Currency: models.CNY},
		{ID: "P0011", ServiceName: "亚伯达省公司年审", Amount: f64(150), Currency: models.CAD},
		{ID: "P0012", ServiceName: "亚伯达省goodstanding", Amount: f64(150), Currency: models.CAD},
		{ID: "P0013", ServiceName: "萨省公司注册", Amount: nil, Currency: models.CAD},
		{ID: "P0014", ServiceName: "萨省公司年审", Amount: nil, Currency: models.CAD},
		{ID: "P0015", ServiceName: "申请所得税号，GST和进出口税号（咱们注册）", Amount: f64(250), Currency: models.CNY},
		{ID: "P0016", ServiceName: "申请所得税号，GST和进出口税号（外来的公）", Amount: f64(500), Currency: models.CNY},
		{ID: "P0017", ServiceName: "外国公司申请BN和GST", Amount: f64(200), Currency: models.CAD},
		{ID: "P0018", ServiceName: "加拿大联邦公司注销", Amount: f64(200), Currency: models.CAD},
		{ID: "P0019", ServiceName: "个人报税，一个人，一个T4", Amount: f64(100), Currency: models.CAD},
		{ID: "P0020", ServiceName: "GST零申报", Amount: f64(50), Currency: models.CAD},
		{ID: "P0021", ServiceName: "有业务GST申报", Amount: f64(100), Currency: models.CAD, Note: "起"},
		{ID: "P0022", ServiceName: "各省PST", Amount: nil, Currency: models.CAD},
	}
}
