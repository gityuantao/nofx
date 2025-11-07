package market

import "time"

// Data 市场数据结构
type Data struct {
	Symbol            string
	CurrentPrice      float64
	PriceChange1h     float64 // 1小时价格变化百分比
	PriceChange4h     float64 // 4小时价格变化百分比
	PriceChange24h    float64 // 24小时价格变化百分比（单日波动率）
	Volatility1h      float64 // 1小时波动率（用于检查是否<1%持续1小时）
	IntegerLevelDistance float64 // 距离最近整数关口的百分比（用于BTC状态检查）
	IsNearIntegerLevel bool   // 是否处于整数关口±2%（用于BTC状态检查）
	IsKeyLevelBreakout bool   // 是否刚突破/跌破关键技术位（用于BTC状态检查）
	CurrentEMA20      float64
	CurrentEMA50      float64 // 3分钟周期EMA50
	CurrentMACD       float64
	CurrentRSI7       float64
	OpenInterest      *OIData
	FundingRate       float64
	BuySellRatio      float64 // 买卖压力比 (TakerBuyVolume / TotalVolume)
	IntradaySeries    *IntradayData
	LongerTermContext *LongerTermData
	// 15分钟周期数据
	EMA20_15m         float64
	EMA50_15m         float64 // 15分钟周期EMA50
	MACD_15m          float64
	RSI_15m           float64
	RSI14_15m         float64 // 15分钟14周期RSI
	Volume_15m        float64 // 15分钟当前成交量
	AvgVolume_15m     float64 // 15分钟平均成交量
	ATR3_15m          float64 // 15分钟3周期ATR
	ATR14_15m         float64 // 15分钟14周期ATR
	// 1小时周期数据
	EMA20_1h          float64
	EMA50_1h          float64 // 1小时周期EMA50
	MACD_1h           float64
	RSI_1h            float64
	RSI14_1h          float64 // 1小时14周期RSI
	Volume_1h         float64 // 1小时当前成交量
	AvgVolume_1h      float64 // 1小时平均成交量
	ATR3_1h           float64 // 1小时3周期ATR
	ATR14_1h          float64 // 1小时14周期ATR
	// 当前K线数据（用于形态分析）
	CurrentKline3m    *Kline // 当前3分钟K线
	CurrentKline15m   *Kline // 当前15分钟K线
	CurrentKline1h    *Kline // 当前1小时K线
}

// OIData Open Interest数据
type OIData struct {
	Latest       float64
	Average      float64
	DeltaPercent float64 // OI持仓量变化百分比
}

// IntradayData 日内数据(3分钟间隔)
type IntradayData struct {
	MidPrices   []float64
	EMA20Values []float64
	MACDValues  []float64
	RSI7Values  []float64
	RSI14Values []float64
	Volumes     []float64 // 成交量序列
	ATR3        float64   // 3周期ATR
	ATR14       float64   // 14周期ATR
	RecentKlines []Kline  // 最近3根K线的完整数据（用于计算实体大小）
}

// LongerTermData 长期数据(4小时时间框架)
type LongerTermData struct {
	EMA20         float64
	EMA50         float64
	ATR3          float64
	ATR14         float64
	CurrentVolume float64
	AverageVolume float64
	MACDValues    []float64
	RSI14Values   []float64
	RecentKlines  []Kline  // 最近5-10根K线（用于形态识别）
	RecentHigh    float64  // 前高（最近一段时间内的最高价）
	RecentLow     float64  // 前低（最近一段时间内的最低价）
	FibonacciLevels []float64 // 斐波那契回撤位（23.6%, 38.2%, 50%, 61.8%, 78.6%）
	ATRUpperBand  float64   // ATR波动带上轨
	ATRLowerBand  float64   // ATR波动带下轨
}

// Binance API 响应结构
type ExchangeInfo struct {
	Symbols []SymbolInfo `json:"symbols"`
}

type SymbolInfo struct {
	Symbol            string `json:"symbol"`
	Status            string `json:"status"`
	BaseAsset         string `json:"baseAsset"`
	QuoteAsset        string `json:"quoteAsset"`
	ContractType      string `json:"contractType"`
	PricePrecision    int    `json:"pricePrecision"`
	QuantityPrecision int    `json:"quantityPrecision"`
}

type Kline struct {
	OpenTime            int64   `json:"openTime"`
	Open                float64 `json:"open"`
	High                float64 `json:"high"`
	Low                 float64 `json:"low"`
	Close               float64 `json:"close"`
	Volume              float64 `json:"volume"`
	CloseTime           int64   `json:"closeTime"`
	QuoteVolume         float64 `json:"quoteVolume"`
	Trades              int     `json:"trades"`
	TakerBuyBaseVolume  float64 `json:"takerBuyBaseVolume"`
	TakerBuyQuoteVolume float64 `json:"takerBuyQuoteVolume"`
}

type KlineResponse []interface{}

type PriceTicker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

type Ticker24hr struct {
	Symbol             string `json:"symbol"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
}

// 特征数据结构
type SymbolFeatures struct {
	Symbol           string    `json:"symbol"`
	Timestamp        time.Time `json:"timestamp"`
	Price            float64   `json:"price"`
	PriceChange15Min float64   `json:"price_change_15min"`
	PriceChange1H    float64   `json:"price_change_1h"`
	PriceChange4H    float64   `json:"price_change_4h"`
	Volume           float64   `json:"volume"`
	VolumeRatio5     float64   `json:"volume_ratio_5"`
	VolumeRatio20    float64   `json:"volume_ratio_20"`
	VolumeTrend      float64   `json:"volume_trend"`
	RSI14            float64   `json:"rsi_14"`
	SMA5             float64   `json:"sma_5"`
	SMA10            float64   `json:"sma_10"`
	SMA20            float64   `json:"sma_20"`
	HighLowRatio     float64   `json:"high_low_ratio"`
	Volatility20     float64   `json:"volatility_20"`
	PositionInRange  float64   `json:"position_in_range"`
}

// 警报数据结构
type Alert struct {
	Type      string    `json:"type"`
	Symbol    string    `json:"symbol"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type Config struct {
	AlertThresholds AlertThresholds `json:"alert_thresholds"`
	UpdateInterval  int             `json:"update_interval"` // seconds
	CleanupConfig   CleanupConfig   `json:"cleanup_config"`
}

type AlertThresholds struct {
	VolumeSpike      float64 `json:"volume_spike"`
	PriceChange15Min float64 `json:"price_change_15min"`
	VolumeTrend      float64 `json:"volume_trend"`
	RSIOverbought    float64 `json:"rsi_overbought"`
	RSIOversold      float64 `json:"rsi_oversold"`
}
type CleanupConfig struct {
	InactiveTimeout   time.Duration `json:"inactive_timeout"`    // 不活跃超时时间
	MinScoreThreshold float64       `json:"min_score_threshold"` // 最低评分阈值
	NoAlertTimeout    time.Duration `json:"no_alert_timeout"`    // 无警报超时时间
	CheckInterval     time.Duration `json:"check_interval"`      // 检查间隔
}

var config = Config{
	AlertThresholds: AlertThresholds{
		VolumeSpike:      3.0,
		PriceChange15Min: 0.05,
		VolumeTrend:      2.0,
		RSIOverbought:    70,
		RSIOversold:      30,
	},
	CleanupConfig: CleanupConfig{
		InactiveTimeout:   30 * time.Minute,
		MinScoreThreshold: 15.0,
		NoAlertTimeout:    20 * time.Minute,
		CheckInterval:     5 * time.Minute,
	},
	UpdateInterval: 60, // 1 minute
}
