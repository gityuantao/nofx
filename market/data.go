package market

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// Get 获取指定代币的市场数据
func Get(symbol string) (*Data, error) {
	var klines3m, klines15m, klines1h, klines4h []Kline
	var err error
	// 标准化symbol
	symbol = Normalize(symbol)
	// 获取3分钟K线数据 (最近10个)
	klines3m, err = WSMonitorCli.GetCurrentKlines(symbol, "3m") // 多获取一些用于计算
	if err != nil {
		return nil, fmt.Errorf("获取3分钟K线失败: %v", err)
	}

	// 获取15分钟K线数据 (最近10个)
	klines15m, err = WSMonitorCli.GetCurrentKlines(symbol, "15m")
	if err != nil {
		// 15m失败不影响整体，使用空数据
		klines15m = []Kline{}
	}

	// 获取1小时K线数据 (最近10个)
	klines1h, err = WSMonitorCli.GetCurrentKlines(symbol, "1h")
	if err != nil {
		// 1h失败不影响整体，使用空数据
		klines1h = []Kline{}
	}

	// 获取4小时K线数据 (最近10个)
	klines4h, err = WSMonitorCli.GetCurrentKlines(symbol, "4h") // 多获取用于计算指标
	if err != nil {
		return nil, fmt.Errorf("获取4小时K线失败: %v", err)
	}

	// 计算当前指标 (基于3分钟最新数据)
	currentPrice := klines3m[len(klines3m)-1].Close
	currentEMA20 := calculateEMA(klines3m, 20)
	currentEMA50 := 0.0
	if len(klines3m) >= 50 {
		currentEMA50 = calculateEMA(klines3m, 50)
	}
	currentMACD := calculateMACD(klines3m)
	currentRSI7 := calculateRSI(klines3m, 7)

	// 计算价格变化百分比
	// 1小时价格变化 = 20个3分钟K线前的价格
	priceChange1h := 0.0
	if len(klines3m) >= 21 { // 至少需要21根K线 (当前 + 20根前)
		price1hAgo := klines3m[len(klines3m)-21].Close
		if price1hAgo > 0 {
			priceChange1h = ((currentPrice - price1hAgo) / price1hAgo) * 100
		}
	}

	// 4小时价格变化 = 1个4小时K线前的价格
	priceChange4h := 0.0
	if len(klines4h) >= 2 {
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			priceChange4h = ((currentPrice - price4hAgo) / price4hAgo) * 100
		}
	}

	// 计算24小时价格变化（单日波动率）
	priceChange24h := 0.0
	// 尝试从1小时K线获取24小时前的价格
	if len(klines1h) >= 25 { // 至少需要25根1小时K线（24小时前 + 当前）
		price24hAgo := klines1h[len(klines1h)-25].Close
		if price24hAgo > 0 {
			priceChange24h = ((currentPrice - price24hAgo) / price24hAgo) * 100
		}
	} else if len(klines4h) >= 7 { // 或者从4小时K线获取（6个4小时K线前 = 24小时前）
		price24hAgo := klines4h[len(klines4h)-7].Close
		if price24hAgo > 0 {
			priceChange24h = ((currentPrice - price24hAgo) / price24hAgo) * 100
		}
	}

	// 计算1小时波动率（用于检查是否<1%持续1小时）
	volatility1h := 0.0
	if len(klines1h) >= 2 {
		// 使用最近2根1小时K线的价格范围计算波动率
		recentHigh := klines1h[len(klines1h)-1].High
		recentLow := klines1h[len(klines1h)-1].Low
		if len(klines1h) >= 2 {
			prevHigh := klines1h[len(klines1h)-2].High
			prevLow := klines1h[len(klines1h)-2].Low
			maxHigh := math.Max(recentHigh, prevHigh)
			minLow := math.Min(recentLow, prevLow)
			if currentPrice > 0 {
				volatility1h = ((maxHigh - minLow) / currentPrice) * 100
			}
		}
	}

	// 计算整数关口距离（用于BTC状态检查）
	integerLevels := calculateIntegerLevels(currentPrice)
	integerLevelDistance := 999.0 // 默认值，表示很远
	isNearIntegerLevel := false
	if len(integerLevels) > 0 {
		// 找到最近的整数关口
		nearestLevel := integerLevels[0]
		integerLevelDistance = math.Abs((currentPrice - nearestLevel) / currentPrice) * 100
		// 检查是否处于整数关口±1%（优化：从±2%收紧为±1%，减少误判）
		// 注意：在宽松模式下，AI会根据提示词规则进一步放宽此限制
		if integerLevelDistance <= 1.0 {
			isNearIntegerLevel = true
		}
	}

	// 判断是否刚突破/跌破关键技术位（用于BTC状态检查）
	isKeyLevelBreakout := false
	// 检查是否刚突破/跌破EMA20或EMA50（价格在最近几个周期内跨越了这些技术位）
	if len(klines1h) >= 3 {
		// 获取1小时周期的EMA20和EMA50
		ema20_1h_check := calculateEMA(klines1h, 20)
		ema50_1h_check := 0.0
		if len(klines1h) >= 50 {
			ema50_1h_check = calculateEMA(klines1h, 50)
		}
		// 检查当前价格和前2根K线的价格是否跨越了技术位
		currentPrice_1h := klines1h[len(klines1h)-1].Close
		prevPrice2 := klines1h[len(klines1h)-3].Close
		// 检查是否刚突破EMA20（从下方突破到上方，或从上方跌破到下方）
		if ema20_1h_check > 0 {
			if (prevPrice2 < ema20_1h_check && currentPrice_1h > ema20_1h_check) ||
			   (prevPrice2 > ema20_1h_check && currentPrice_1h < ema20_1h_check) {
				isKeyLevelBreakout = true
			}
		}
		// 检查是否刚突破EMA50
		if ema50_1h_check > 0 && !isKeyLevelBreakout {
			if (prevPrice2 < ema50_1h_check && currentPrice_1h > ema50_1h_check) ||
			   (prevPrice2 > ema50_1h_check && currentPrice_1h < ema50_1h_check) {
				isKeyLevelBreakout = true
			}
		}
		// 检查是否刚突破整数关口
		if len(integerLevels) > 0 && !isKeyLevelBreakout {
			nearestLevel := integerLevels[0]
			if integerLevelDistance <= 0.5 { // 距离整数关口<0.5%，可能刚突破
				if (prevPrice2 < nearestLevel && currentPrice_1h > nearestLevel) ||
				   (prevPrice2 > nearestLevel && currentPrice_1h < nearestLevel) {
					isKeyLevelBreakout = true
				}
			}
		}
	}

	// 计算15分钟周期指标
	ema20_15m := 0.0
	ema50_15m := 0.0
	macd_15m := 0.0
	rsi_15m := 0.0
	rsi14_15m := 0.0
	volume_15m := 0.0
	avgVolume_15m := 0.0
	atr3_15m := 0.0
	atr14_15m := 0.0
	if len(klines15m) >= 20 {
		ema20_15m = calculateEMA(klines15m, 20)
		if len(klines15m) >= 50 {
			ema50_15m = calculateEMA(klines15m, 50)
		}
		macd_15m = calculateMACD(klines15m)
		rsi_15m = calculateRSI(klines15m, 7)
		if len(klines15m) >= 14 {
			rsi14_15m = calculateRSI(klines15m, 14)
		}
		// 计算成交量
		if len(klines15m) > 0 {
			volume_15m = klines15m[len(klines15m)-1].Volume
			sum := 0.0
			for _, k := range klines15m {
				sum += k.Volume
			}
			avgVolume_15m = sum / float64(len(klines15m))
		}
		// 计算ATR
		if len(klines15m) >= 3 {
			atr3_15m = calculateATR(klines15m, 3)
		}
		if len(klines15m) >= 14 {
			atr14_15m = calculateATR(klines15m, 14)
		}
	}

	// 计算1小时周期指标
	ema20_1h := 0.0
	ema50_1h := 0.0
	macd_1h := 0.0
	rsi_1h := 0.0
	rsi14_1h := 0.0
	volume_1h := 0.0
	avgVolume_1h := 0.0
	atr3_1h := 0.0
	atr14_1h := 0.0
	if len(klines1h) >= 20 {
		ema20_1h = calculateEMA(klines1h, 20)
		if len(klines1h) >= 50 {
			ema50_1h = calculateEMA(klines1h, 50)
		}
		macd_1h = calculateMACD(klines1h)
		rsi_1h = calculateRSI(klines1h, 7)
		if len(klines1h) >= 14 {
			rsi14_1h = calculateRSI(klines1h, 14)
		}
		// 计算成交量
		if len(klines1h) > 0 {
			volume_1h = klines1h[len(klines1h)-1].Volume
			sum := 0.0
			for _, k := range klines1h {
				sum += k.Volume
			}
			avgVolume_1h = sum / float64(len(klines1h))
		}
		// 计算ATR
		if len(klines1h) >= 3 {
			atr3_1h = calculateATR(klines1h, 3)
		}
		if len(klines1h) >= 14 {
			atr14_1h = calculateATR(klines1h, 14)
		}
	}

	// 获取OI数据
	oiData, err := getOpenInterestData(symbol)
	if err != nil {
		// OI失败不影响整体,使用默认值
		oiData = &OIData{Latest: 0, Average: 0, DeltaPercent: 0}
	} else {
		// 计算OI持仓量变化百分比
		if oiData.Average > 0 {
			oiData.DeltaPercent = ((oiData.Latest - oiData.Average) / oiData.Average) * 100
		}
	}

	// 获取Funding Rate
	fundingRate, _ := getFundingRate(symbol)

	// 获取BuySellRatio（买卖压力比）
	buySellRatio := getBuySellRatio(klines3m)

	// 计算日内系列数据
	intradayData := calculateIntradaySeries(klines3m)

	// 计算长期数据
	longerTermData := calculateLongerTermData(klines4h)

	// 获取当前K线数据（用于形态分析）
	var currentKline3m, currentKline15m, currentKline1h *Kline
	if len(klines3m) > 0 {
		k := klines3m[len(klines3m)-1]
		currentKline3m = &k
	}
	if len(klines15m) > 0 {
		k := klines15m[len(klines15m)-1]
		currentKline15m = &k
	}
	if len(klines1h) > 0 {
		k := klines1h[len(klines1h)-1]
		currentKline1h = &k
	}

	return &Data{
		Symbol:                symbol,
		CurrentPrice:          currentPrice,
		PriceChange1h:         priceChange1h,
		PriceChange4h:         priceChange4h,
		PriceChange24h:        priceChange24h,
		Volatility1h:          volatility1h,
		IntegerLevelDistance:  integerLevelDistance,
		IsNearIntegerLevel:    isNearIntegerLevel,
		IsKeyLevelBreakout:    isKeyLevelBreakout,
		CurrentEMA20:          currentEMA20,
		CurrentEMA50:          currentEMA50,
		CurrentMACD:           currentMACD,
		CurrentRSI7:           currentRSI7,
		OpenInterest:      oiData,
		FundingRate:       fundingRate,
		BuySellRatio:      buySellRatio,
		IntradaySeries:    intradayData,
		LongerTermContext: longerTermData,
		EMA20_15m:         ema20_15m,
		EMA50_15m:         ema50_15m,
		MACD_15m:          macd_15m,
		RSI_15m:           rsi_15m,
		RSI14_15m:         rsi14_15m,
		Volume_15m:        volume_15m,
		AvgVolume_15m:     avgVolume_15m,
		ATR3_15m:          atr3_15m,
		ATR14_15m:         atr14_15m,
		EMA20_1h:          ema20_1h,
		EMA50_1h:          ema50_1h,
		MACD_1h:           macd_1h,
		RSI_1h:            rsi_1h,
		RSI14_1h:          rsi14_1h,
		Volume_1h:         volume_1h,
		AvgVolume_1h:      avgVolume_1h,
		ATR3_1h:           atr3_1h,
		ATR14_1h:          atr14_1h,
		CurrentKline3m:    currentKline3m,
		CurrentKline15m:   currentKline15m,
		CurrentKline1h:    currentKline1h,
	}, nil
}

// calculateEMA 计算EMA
func calculateEMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}

	// 计算SMA作为初始EMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)

	// 计算EMA
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}

	return ema
}

// calculateMACD 计算MACD
func calculateMACD(klines []Kline) float64 {
	if len(klines) < 26 {
		return 0
	}

	// 计算12期和26期EMA
	ema12 := calculateEMA(klines, 12)
	ema26 := calculateEMA(klines, 26)

	// MACD = EMA12 - EMA26
	return ema12 - ema26
}

// calculateRSI 计算RSI
func calculateRSI(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	gains := 0.0
	losses := 0.0

	// 计算初始平均涨跌幅
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// 使用Wilder平滑方法计算后续RSI
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + (-change)) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateATR 计算ATR
func calculateATR(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	trs := make([]float64, len(klines))
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// 计算初始ATR
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += trs[i]
	}
	atr := sum / float64(period)

	// Wilder平滑
	for i := period + 1; i < len(klines); i++ {
		atr = (atr*float64(period-1) + trs[i]) / float64(period)
	}

	return atr
}

// calculateIntradaySeries 计算日内系列数据
func calculateIntradaySeries(klines []Kline) *IntradayData {
	data := &IntradayData{
		MidPrices:   make([]float64, 0, 10),
		EMA20Values: make([]float64, 0, 10),
		MACDValues:  make([]float64, 0, 10),
		RSI7Values:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
		Volumes:     make([]float64, 0, 10),
	}

	// 获取最近10个数据点
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		data.MidPrices = append(data.MidPrices, klines[i].Close)
		data.Volumes = append(data.Volumes, klines[i].Volume)

		// 计算每个点的EMA20
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// 计算每个点的MACD
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// 计算每个点的RSI
		if i >= 7 {
			rsi7 := calculateRSI(klines[:i+1], 7)
			data.RSI7Values = append(data.RSI7Values, rsi7)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	// 计算ATR
	if len(klines) >= 3 {
		data.ATR3 = calculateATR(klines, 3)
	}
	if len(klines) >= 14 {
		data.ATR14 = calculateATR(klines, 14)
	}

	// 保存最近3根K线的完整数据（用于计算实体大小）
	data.RecentKlines = make([]Kline, 0, 3)
	if len(klines) >= 3 {
		// 获取最后3根K线
		for i := len(klines) - 3; i < len(klines); i++ {
			if i >= 0 {
				data.RecentKlines = append(data.RecentKlines, klines[i])
			}
		}
	}

	return data
}

// calculateLongerTermData 计算长期数据
func calculateLongerTermData(klines []Kline) *LongerTermData {
	data := &LongerTermData{
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 计算EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)

	// 计算ATR
	data.ATR3 = calculateATR(klines, 3)
	data.ATR14 = calculateATR(klines, 14)

	// 计算成交量
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume
		// 计算平均成交量
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// 计算MACD和RSI序列
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	// 保存最近5-10根K线（用于形态识别）
	data.RecentKlines = make([]Kline, 0, 10)
	recentStart := len(klines) - 10
	if recentStart < 0 {
		recentStart = 0
	}
	for i := recentStart; i < len(klines); i++ {
		data.RecentKlines = append(data.RecentKlines, klines[i])
	}

	// 计算前高/前低（最近20根K线）
	data.RecentHigh = 0.0
	data.RecentLow = 999999999.0
	lookback := 20
	if len(klines) < lookback {
		lookback = len(klines)
	}
	for i := len(klines) - lookback; i < len(klines); i++ {
		if i >= 0 {
			if klines[i].High > data.RecentHigh {
				data.RecentHigh = klines[i].High
			}
			if klines[i].Low < data.RecentLow {
				data.RecentLow = klines[i].Low
			}
		}
	}
	if data.RecentLow == 999999999.0 {
		data.RecentLow = 0.0
	}

	// 计算斐波那契回撤位（基于前高和前低）
	data.FibonacciLevels = make([]float64, 0, 5)
	if data.RecentHigh > 0 && data.RecentLow > 0 && data.RecentHigh > data.RecentLow {
		rangeSize := data.RecentHigh - data.RecentLow
		// 计算回撤位（从高到低）
		fibLevels := []float64{0.236, 0.382, 0.5, 0.618, 0.786}
		for _, fib := range fibLevels {
			level := data.RecentHigh - (rangeSize * fib)
			data.FibonacciLevels = append(data.FibonacciLevels, level)
		}
	}

	// 计算ATR波动带（基于当前价格和ATR14）
	currentPrice := klines[len(klines)-1].Close
	if data.ATR14 > 0 {
		// 上轨 = 当前价格 + ATR14 × 2
		data.ATRUpperBand = currentPrice + (data.ATR14 * 2)
		// 下轨 = 当前价格 - ATR14 × 2
		data.ATRLowerBand = currentPrice - (data.ATR14 * 2)
	}

	return data
}

// getOpenInterestData 获取OI数据
func getOpenInterestData(symbol string) (*OIData, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/openInterest?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		OpenInterest string `json:"openInterest"`
		Symbol       string `json:"symbol"`
		Time         int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	oi, _ := strconv.ParseFloat(result.OpenInterest, 64)

	// 计算变化百分比（使用近似平均值）
	deltaPercent := 0.0
	average := oi * 0.999 // 近似平均值
	if average > 0 {
		deltaPercent = ((oi - average) / average) * 100
	}

	return &OIData{
		Latest:       oi,
		Average:      average,
		DeltaPercent: deltaPercent,
	}, nil
}

// getBuySellRatio 计算买卖压力比 (TakerBuyVolume / TotalVolume)
func getBuySellRatio(klines []Kline) float64 {
	if len(klines) == 0 {
		return 0.5 // 默认中性值
	}

	// 使用最近几根K线的数据计算
	lookback := 5
	if len(klines) < lookback {
		lookback = len(klines)
	}

	totalBuyVolume := 0.0
	totalVolume := 0.0

	for i := len(klines) - lookback; i < len(klines); i++ {
		if i >= 0 {
			totalBuyVolume += klines[i].TakerBuyBaseVolume
			totalVolume += klines[i].Volume
		}
	}

	if totalVolume > 0 {
		return totalBuyVolume / totalVolume
	}

	return 0.5 // 默认中性值
}

// getFundingRate 获取资金费率
func getFundingRate(symbol string) (float64, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/premiumIndex?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Symbol          string `json:"symbol"`
		MarkPrice       string `json:"markPrice"`
		IndexPrice      string `json:"indexPrice"`
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
		InterestRate    string `json:"interestRate"`
		Time            int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	rate, _ := strconv.ParseFloat(result.LastFundingRate, 64)
	return rate, nil
}

// getMACDDirection 获取MACD方向描述
func getMACDDirection(macd float64) string {
	if macd > 0 {
		return "(多头)"
	} else if macd < 0 {
		return "(空头)"
	}
	return "(中性)"
}

// Format 格式化输出市场数据
func Format(data *Data) string {
	var sb strings.Builder

	// 3分钟周期（当前周期）
	sb.WriteString("=== 3分钟周期 ===\n")
	priceVsEMA3m := "价格 < EMA20"
	if data.CurrentPrice > data.CurrentEMA20 {
		priceVsEMA3m = "价格 > EMA20"
	} else if data.CurrentPrice == data.CurrentEMA20 {
		priceVsEMA3m = "价格 = EMA20"
	}
	sb.WriteString(fmt.Sprintf("current_price = %.2f, current_ema20 = %.3f", data.CurrentPrice, data.CurrentEMA20))
	if data.CurrentEMA50 > 0 {
		sb.WriteString(fmt.Sprintf(", current_ema50 = %.3f", data.CurrentEMA50))
	}
	sb.WriteString(fmt.Sprintf(", %s, current_macd = %.3f, current_rsi (7 period) = %.3f",
		priceVsEMA3m, data.CurrentMACD, data.CurrentRSI7))
	// 添加3分钟周期的RSI14、成交量和ATR
	if data.IntradaySeries != nil && len(data.IntradaySeries.RSI14Values) > 0 {
		rsi14_3m := data.IntradaySeries.RSI14Values[len(data.IntradaySeries.RSI14Values)-1]
		sb.WriteString(fmt.Sprintf(", current_rsi (14 period) = %.3f", rsi14_3m))
	}
	if data.IntradaySeries != nil && len(data.IntradaySeries.Volumes) > 0 {
		volume_3m := data.IntradaySeries.Volumes[len(data.IntradaySeries.Volumes)-1]
		// 计算平均成交量
		avgVolume_3m := 0.0
		if len(data.IntradaySeries.Volumes) > 0 {
			sum := 0.0
			for _, v := range data.IntradaySeries.Volumes {
				sum += v
			}
			avgVolume_3m = sum / float64(len(data.IntradaySeries.Volumes))
		}
		if avgVolume_3m > 0 {
			volumeRatio := volume_3m / avgVolume_3m
			sb.WriteString(fmt.Sprintf(" | 成交量: %.3f (均量: %.3f, 比率: %.2fx)", volume_3m, avgVolume_3m, volumeRatio))
		}
	}
	if data.IntradaySeries != nil && data.IntradaySeries.ATR3 > 0 {
		sb.WriteString(fmt.Sprintf(" | ATR(3): %.3f", data.IntradaySeries.ATR3))
	}
	if data.IntradaySeries != nil && data.IntradaySeries.ATR14 > 0 {
		sb.WriteString(fmt.Sprintf(" | ATR(14): %.3f", data.IntradaySeries.ATR14))
	}
	sb.WriteString("\n\n")

	// 15分钟周期（只要有任一指标有值就显示）
	if data.EMA20_15m != 0 || data.MACD_15m != 0 || data.RSI_15m > 0 {
		sb.WriteString("=== 15分钟周期 ===\n")
		priceVsEMA15m := "价格 < EMA20"
		if data.CurrentPrice > data.EMA20_15m {
			priceVsEMA15m = "价格 > EMA20"
		} else if data.CurrentPrice == data.EMA20_15m {
			priceVsEMA15m = "价格 = EMA20"
		}
		sb.WriteString(fmt.Sprintf("价格: %.2f | EMA20: %.3f", data.CurrentPrice, data.EMA20_15m))
		if data.EMA50_15m > 0 {
			sb.WriteString(fmt.Sprintf(" | EMA50: %.3f", data.EMA50_15m))
		}
		sb.WriteString(fmt.Sprintf(" | %s | MACD: %.3f %s | RSI(7): %.2f",
			priceVsEMA15m, data.MACD_15m, getMACDDirection(data.MACD_15m), data.RSI_15m))
		if data.RSI14_15m > 0 {
			sb.WriteString(fmt.Sprintf(" | RSI(14): %.2f", data.RSI14_15m))
		}
		if data.Volume_15m > 0 && data.AvgVolume_15m > 0 {
			volumeRatio := data.Volume_15m / data.AvgVolume_15m
			sb.WriteString(fmt.Sprintf(" | 成交量: %.3f (均量: %.3f, 比率: %.2fx)", data.Volume_15m, data.AvgVolume_15m, volumeRatio))
		}
		if data.ATR3_15m > 0 {
			sb.WriteString(fmt.Sprintf(" | ATR(3): %.3f", data.ATR3_15m))
		}
		if data.ATR14_15m > 0 {
			sb.WriteString(fmt.Sprintf(" | ATR(14): %.3f", data.ATR14_15m))
		}
		sb.WriteString("\n\n")
	}

	// 1小时周期（只要有任一指标有值就显示）
	if data.EMA20_1h != 0 || data.MACD_1h != 0 || data.RSI_1h > 0 {
		sb.WriteString("=== 1小时周期 ===\n")
		priceVsEMA1h := "价格 < EMA20"
		if data.CurrentPrice > data.EMA20_1h {
			priceVsEMA1h = "价格 > EMA20"
		} else if data.CurrentPrice == data.EMA20_1h {
			priceVsEMA1h = "价格 = EMA20"
		}
		sb.WriteString(fmt.Sprintf("价格: %.2f | EMA20: %.3f", data.CurrentPrice, data.EMA20_1h))
		if data.EMA50_1h > 0 {
			sb.WriteString(fmt.Sprintf(" | EMA50: %.3f", data.EMA50_1h))
		}
		sb.WriteString(fmt.Sprintf(" | %s | MACD: %.3f %s | RSI(7): %.2f",
			priceVsEMA1h, data.MACD_1h, getMACDDirection(data.MACD_1h), data.RSI_1h))
		if data.RSI14_1h > 0 {
			sb.WriteString(fmt.Sprintf(" | RSI(14): %.2f", data.RSI14_1h))
		}
		if data.Volume_1h > 0 && data.AvgVolume_1h > 0 {
			volumeRatio := data.Volume_1h / data.AvgVolume_1h
			sb.WriteString(fmt.Sprintf(" | 成交量: %.3f (均量: %.3f, 比率: %.2fx)", data.Volume_1h, data.AvgVolume_1h, volumeRatio))
		}
		if data.ATR3_1h > 0 {
			sb.WriteString(fmt.Sprintf(" | ATR(3): %.3f", data.ATR3_1h))
		}
		if data.ATR14_1h > 0 {
			sb.WriteString(fmt.Sprintf(" | ATR(14): %.3f", data.ATR14_1h))
		}
		sb.WriteString("\n\n")
	}

	// 4小时周期（在LongerTermContext中）
	sb.WriteString(fmt.Sprintf("In addition, here is the latest %s open interest and funding rate for perps:\n\n",
		data.Symbol))

	if data.OpenInterest != nil {
		sb.WriteString(fmt.Sprintf("Open Interest: Latest: %.2f Average: %.2f", data.OpenInterest.Latest, data.OpenInterest.Average))
		if data.OpenInterest.DeltaPercent != 0 {
			sb.WriteString(fmt.Sprintf(" | 变化: %+.2f%%", data.OpenInterest.DeltaPercent))
			// 明确标注是否 >+5%
			if data.OpenInterest.DeltaPercent > 5.0 {
				sb.WriteString(" ✅ **OI持仓量变化 >+5%（真实突破确认）**")
			} else {
				sb.WriteString(" ⚠️ OI持仓量变化 ≤+5%")
			}
		}
		sb.WriteString("\n\n")
	}

	// 资金费率（优化格式）
	sb.WriteString(fmt.Sprintf("Funding Rate: %.6f (%.4f%%)\n\n", data.FundingRate, data.FundingRate*100))

	// BuySellRatio（买卖压力比）
	sb.WriteString(fmt.Sprintf("BuySellRatio: %.3f", data.BuySellRatio))
	if data.BuySellRatio > 0.7 {
		sb.WriteString(" (强买)")
	} else if data.BuySellRatio > 0.55 {
		sb.WriteString(" (偏多)")
	} else if data.BuySellRatio < 0.3 {
		sb.WriteString(" (强卖)")
	} else if data.BuySellRatio < 0.45 {
		sb.WriteString(" (偏空)")
	} else {
		sb.WriteString(" (中性)")
	}
	sb.WriteString("\n\n")

	// 当前K线形态数据（用于防假突破检测）
	if data.CurrentKline3m != nil {
		k := data.CurrentKline3m
		body := math.Abs(k.Close - k.Open)
		totalRange := k.High - k.Low
		upperShadow := k.High - math.Max(k.Open, k.Close)
		lowerShadow := math.Min(k.Open, k.Close) - k.Low
		
		sb.WriteString("=== 当前3分钟K线形态 ===\n")
		sb.WriteString(fmt.Sprintf("OHLC: Open=%.2f, High=%.2f, Low=%.2f, Close=%.2f\n", k.Open, k.High, k.Low, k.Close))
		sb.WriteString(fmt.Sprintf("上影: %.2f | 下影: %.2f | 实体: %.2f | 总长度: %.2f\n", upperShadow, lowerShadow, body, totalRange))
		
		// 形态判断提示
		if totalRange > 0 {
			if body/totalRange < 0.2 {
				sb.WriteString("⚠️ 十字星形态（实体 < 总长度 × 0.2）\n")
			}
			if upperShadow > body*2 && k.Close < k.Open {
				sb.WriteString("⚠️ 长上影（上影 > 实体 × 2，上方抛压大）\n")
			}
			if lowerShadow > body*2 && k.Close > k.Open {
				sb.WriteString("⚠️ 长下影（下影 > 实体 × 2，下方承接力强）\n")
			}
		}
		
		// 计算最近3根K线的实体大小（用于防假突破检测）
		if data.IntradaySeries != nil && len(data.IntradaySeries.RecentKlines) >= 3 {
			sb.WriteString("\n最近3根K线实体大小检查（用于防假突破检测）:\n")
			allSmall := true
			for i, k := range data.IntradaySeries.RecentKlines {
				body := math.Abs(k.Close - k.Open)
				atr := data.IntradaySeries.ATR3
				if atr == 0 {
					atr = data.IntradaySeries.ATR14
				}
				if atr > 0 {
					bodyRatio := body / atr
					sb.WriteString(fmt.Sprintf("  K线%d: 实体=%.2f, ATR=%.2f, 比率=%.2f", i+1, body, atr, bodyRatio))
					if bodyRatio < 0.3 {
						sb.WriteString(" ⚠️ 实体极小（< ATR × 0.3）\n")
					} else {
						sb.WriteString(" ✅ 实体正常\n")
						allSmall = false
					}
				} else {
					sb.WriteString(fmt.Sprintf("  K线%d: 实体=%.2f (ATR数据不足，无法判断)\n", i+1, body))
					allSmall = false
				}
			}
			if allSmall && data.IntradaySeries.ATR3 > 0 {
				sb.WriteString("❌ **连续3根K线实体极小（实体 < ATR × 0.3）→ 波动率下降，无趋势**\n")
			}
		}
		sb.WriteString("\n")
	}

	if data.IntradaySeries != nil {
		sb.WriteString("Intraday series (3‑minute intervals, oldest → latest):\n\n")

		if len(data.IntradaySeries.MidPrices) > 0 {
			sb.WriteString(fmt.Sprintf("Mid prices: %s\n\n", formatFloatSlice(data.IntradaySeries.MidPrices)))
		}

		if len(data.IntradaySeries.EMA20Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA indicators (20‑period): %s\n\n", formatFloatSlice(data.IntradaySeries.EMA20Values)))
		}

		if len(data.IntradaySeries.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD indicators: %s", formatFloatSlice(data.IntradaySeries.MACDValues)))
			// 判断MACD死叉状态（用于做空策略）
			if len(data.IntradaySeries.MACDValues) >= 2 {
				prevMACD := data.IntradaySeries.MACDValues[len(data.IntradaySeries.MACDValues)-2]
				currMACD := data.IntradaySeries.MACDValues[len(data.IntradaySeries.MACDValues)-1]
				// 死叉：从正转负，或从上方穿越信号线（这里简化为从正转负）
				if prevMACD > 0 && currMACD < 0 {
					sb.WriteString(" ⚠️ **MACD死叉（从正转负）**")
				} else if prevMACD < 0 && currMACD > 0 {
					sb.WriteString(" ✅ MACD金叉（从负转正）")
				} else if currMACD < 0 {
					sb.WriteString(" (空头)")
				} else {
					sb.WriteString(" (多头)")
				}
			}
			sb.WriteString("\n\n")
		}

		if len(data.IntradaySeries.RSI7Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (7‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI7Values)))
		}

		if len(data.IntradaySeries.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI14Values)))
		}

		if len(data.IntradaySeries.Volumes) > 0 {
			sb.WriteString(fmt.Sprintf("Volume indicators: %s\n\n", formatFloatSlice(data.IntradaySeries.Volumes)))
		}

		if data.IntradaySeries.ATR3 > 0 {
			sb.WriteString(fmt.Sprintf("ATR (3‑Period): %.3f\n\n", data.IntradaySeries.ATR3))
		}
		if data.IntradaySeries.ATR14 > 0 {
			sb.WriteString(fmt.Sprintf("ATR (14‑Period): %.3f\n\n", data.IntradaySeries.ATR14))
		}
	}

	if data.LongerTermContext != nil {
		sb.WriteString("=== 4小时周期 ===\n")
		
		// 价格与EMA20关系
		priceVsEMA4h := "价格 < EMA20"
		if data.CurrentPrice > data.LongerTermContext.EMA20 {
			priceVsEMA4h = "价格 > EMA20"
		} else if data.CurrentPrice == data.LongerTermContext.EMA20 {
			priceVsEMA4h = "价格 = EMA20"
		}
		
		// 4小时MACD（最新值）
		macd4h := 0.0
		if len(data.LongerTermContext.MACDValues) > 0 {
			macd4h = data.LongerTermContext.MACDValues[len(data.LongerTermContext.MACDValues)-1]
		}
		
		// 4小时RSI（最新值）
		rsi4h := 0.0
		if len(data.LongerTermContext.RSI14Values) > 0 {
			rsi4h = data.LongerTermContext.RSI14Values[len(data.LongerTermContext.RSI14Values)-1]
		}
		
		sb.WriteString(fmt.Sprintf("价格: %.2f | EMA20: %.3f | %s | MACD: %.3f %s | RSI: %.2f\n\n",
			data.CurrentPrice, data.LongerTermContext.EMA20, priceVsEMA4h, macd4h, getMACDDirection(macd4h), rsi4h))
		
		// 技术位信息（支撑/阻力/整数关口）
		sb.WriteString("### 技术位信息\n\n")
		
		// 整数关口（基于当前价格计算）
		integerLevels := calculateIntegerLevels(data.CurrentPrice)
		if len(integerLevels) > 0 {
			sb.WriteString("整数关口: ")
			nearestLevelDistance := 999.0
			for i, level := range integerLevels {
				if i > 0 {
					sb.WriteString(", ")
				}
				distance := ((data.CurrentPrice - level) / data.CurrentPrice) * 100
				if distance < 0 {
					distance = -distance
				}
				if i == 0 {
					nearestLevelDistance = distance
				}
				// 根据价格范围选择合适的格式
				if level >= 1 {
					sb.WriteString(fmt.Sprintf("%.0f (距离%.2f%%)", level, distance))
				} else if level >= 0.1 {
					sb.WriteString(fmt.Sprintf("%.1f (距离%.2f%%)", level, distance))
				} else {
					sb.WriteString(fmt.Sprintf("%.2f (距离%.2f%%)", level, distance))
				}
			}
			// 标注技术位距离是否 <0.5%
			if nearestLevelDistance < 0.5 {
				sb.WriteString(" ❌ **技术位距离 <0.5%（技术位不清晰）**")
			} else {
				sb.WriteString(" ✅ 技术位距离 ≥0.5%")
			}
			sb.WriteString("\n\n")
		}
		
		// 关键技术位（EMA20/EMA50）
		sb.WriteString(fmt.Sprintf("关键技术位: EMA20=%.3f, EMA50=%.3f", data.LongerTermContext.EMA20, data.LongerTermContext.EMA50))
		// 计算最近技术位距离
		ema20Distance := ((data.CurrentPrice - data.LongerTermContext.EMA20) / data.CurrentPrice) * 100
		if ema20Distance < 0 {
			ema20Distance = -ema20Distance
		}
		ema50Distance := ((data.CurrentPrice - data.LongerTermContext.EMA50) / data.CurrentPrice) * 100
		if ema50Distance < 0 {
			ema50Distance = -ema50Distance
		}
		minTechDistance := ema20Distance
		if ema50Distance < minTechDistance {
			minTechDistance = ema50Distance
		}
		if minTechDistance < 0.5 {
			sb.WriteString(" ❌ **技术位距离 <0.5%（技术位不清晰）**")
		} else {
			sb.WriteString(" ✅ 技术位距离 ≥0.5%")
		}
		sb.WriteString("\n\n")

		// 前高/前低（用于设置止盈/止损）
		if data.LongerTermContext.RecentHigh > 0 && data.LongerTermContext.RecentLow > 0 {
			sb.WriteString(fmt.Sprintf("前高/前低: 前高=%.3f, 前低=%.3f (基于最近20根4小时K线)\n\n",
				data.LongerTermContext.RecentHigh, data.LongerTermContext.RecentLow))
		}

		// 斐波那契回撤位（用于设置止盈/止损）
		if len(data.LongerTermContext.FibonacciLevels) > 0 {
			sb.WriteString("斐波那契回撤位（基于前高/前低）:\n")
			fibLabels := []string{"23.6%", "38.2%", "50%", "61.8%", "78.6%"}
			for i, level := range data.LongerTermContext.FibonacciLevels {
				distance := ((data.CurrentPrice - level) / data.CurrentPrice) * 100
				if distance < 0 {
					distance = -distance
				}
				sb.WriteString(fmt.Sprintf("  %s: %.3f (距离当前价格 %.2f%%)\n", fibLabels[i], level, distance))
			}
			sb.WriteString("\n")
		}

		// ATR波动带（用于设置止盈/止损）
		if data.LongerTermContext.ATRUpperBand > 0 && data.LongerTermContext.ATRLowerBand > 0 {
			sb.WriteString(fmt.Sprintf("ATR波动带（基于ATR14 × 2）: 上轨=%.3f, 下轨=%.3f\n",
				data.LongerTermContext.ATRUpperBand, data.LongerTermContext.ATRLowerBand))
			upperDistance := ((data.LongerTermContext.ATRUpperBand - data.CurrentPrice) / data.CurrentPrice) * 100
			lowerDistance := ((data.CurrentPrice - data.LongerTermContext.ATRLowerBand) / data.CurrentPrice) * 100
			sb.WriteString(fmt.Sprintf("  当前价格距离上轨: %.2f%%, 距离下轨: %.2f%%\n\n",
				upperDistance, lowerDistance))
		}

		sb.WriteString(fmt.Sprintf("20‑Period EMA: %.3f vs. 50‑Period EMA: %.3f\n\n",
			data.LongerTermContext.EMA20, data.LongerTermContext.EMA50))

		sb.WriteString(fmt.Sprintf("3‑Period ATR: %.3f vs. 14‑Period ATR: %.3f\n\n",
			data.LongerTermContext.ATR3, data.LongerTermContext.ATR14))

		// 成交量数据（计算比率）
		volumeRatio := 0.0
		if data.LongerTermContext.AverageVolume > 0 {
			volumeRatio = data.LongerTermContext.CurrentVolume / data.LongerTermContext.AverageVolume
		}
		volumeStatus := ""
		if volumeRatio > 1.5 {
			volumeStatus = " (放量 >1.5x)"
		} else if volumeRatio < 0.7 {
			volumeStatus = " ❌ **成交量萎缩（<0.7x）**"
		} else if volumeRatio < 0.8 {
			volumeStatus = " (萎缩 <0.8x)"
		} else {
			volumeStatus = " ✅ 成交量正常（≥0.7x）"
		}
		sb.WriteString(fmt.Sprintf("Current Volume: %.3f vs. Average Volume: %.3f | 比率: %.2fx%s\n\n",
			data.LongerTermContext.CurrentVolume, data.LongerTermContext.AverageVolume, volumeRatio, volumeStatus))

		if len(data.LongerTermContext.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.LongerTermContext.MACDValues)))
		}

		if len(data.LongerTermContext.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.LongerTermContext.RSI14Values)))
		}
		
		// 多周期趋势一致性标注（用于第6步：多周期趋势确认）
		sb.WriteString("### 多周期趋势一致性分析（用于第6步：多周期趋势确认）\n\n")
		
		// 收集4个周期的趋势方向（价格vsEMA20和MACD方向）
		type CycleTrend struct {
			name      string
			priceVsEMA bool  // true=价格>EMA20, false=价格<EMA20
			macd       float64
		}
		
		cycles := []CycleTrend{}
		
		// 3分钟周期
		priceVsEMA3m := data.CurrentPrice > data.CurrentEMA20
		macd3m := data.CurrentMACD
		cycles = append(cycles, CycleTrend{"3m", priceVsEMA3m, macd3m})
		
		// 15分钟周期
		priceVsEMA15m := data.CurrentPrice > data.EMA20_15m
		macd15m := data.MACD_15m
		cycles = append(cycles, CycleTrend{"15m", priceVsEMA15m, macd15m})
		
		// 1小时周期
		priceVsEMA1h := data.CurrentPrice > data.EMA20_1h
		macd1h := data.MACD_1h
		cycles = append(cycles, CycleTrend{"1h", priceVsEMA1h, macd1h})
		
		// 4小时周期（macd4h已在前面定义，这里直接使用）
		priceVsEMA4hBool := data.CurrentPrice > data.LongerTermContext.EMA20
		cycles = append(cycles, CycleTrend{"4h", priceVsEMA4hBool, macd4h})
		
		// 判断做多趋势（价格>EMA20且MACD>0）
		longCount := 0
		for _, c := range cycles {
			if c.priceVsEMA && c.macd > 0 {
				longCount++
			}
		}
		
		// 判断做空趋势（价格<EMA20且MACD<0）
		shortCount := 0
		for _, c := range cycles {
			if !c.priceVsEMA && c.macd < 0 {
				shortCount++
			}
		}
		
		// 输出趋势一致性标注
		if longCount == 4 {
			sb.WriteString("✅ **4个周期全部同向（做多）**：趋势极强（信心 +10）\n")
		} else if shortCount == 4 {
			sb.WriteString("✅ **4个周期全部同向（做空）**：趋势极强（信心 +10）\n")
		} else if longCount == 3 {
			sb.WriteString("✅ **3个周期同向（做多）**：趋势确认（信心 +5）\n")
		} else if shortCount == 3 {
			sb.WriteString("✅ **3个周期同向（做空）**：趋势确认（信心 +5）\n")
		} else if longCount == 2 {
			sb.WriteString("⚠️ **仅2个周期同向（做多）**：趋势可接受（允许开仓，但信心度需 ≥90）\n")
		} else if shortCount == 2 {
			sb.WriteString("⚠️ **仅2个周期同向（做空）**：趋势可接受（允许开仓，但信心度需 ≥90）\n")
		} else {
			sb.WriteString("❌ **多周期趋势不一致**：必须等待趋势共振信号再开仓\n")
		}
		
		// 详细列出各周期状态
		sb.WriteString("\n各周期详细状态：\n")
		for _, c := range cycles {
			priceStatus := "价格 < EMA20"
			if c.priceVsEMA {
				priceStatus = "价格 > EMA20"
			}
			macdStatus := "空头"
			if c.macd > 0 {
				macdStatus = "多头"
			} else if c.macd == 0 {
				macdStatus = "中性"
			}
			sb.WriteString(fmt.Sprintf("  - %s周期: %s, MACD=%.3f (%s)\n", c.name, priceStatus, c.macd, macdStatus))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatFloatSlice 格式化float64切片为字符串
func formatFloatSlice(values []float64) string {
	strValues := make([]string, len(values))
	for i, v := range values {
		strValues[i] = fmt.Sprintf("%.3f", v)
	}
	return "[" + strings.Join(strValues, ", ") + "]"
}

// Normalize 标准化symbol,确保是USDT交易对
func Normalize(symbol string) string {
	symbol = strings.ToUpper(symbol)
	if strings.HasSuffix(symbol, "USDT") {
		return symbol
	}
	return symbol + "USDT"
}

// parseFloat 解析float值
func parseFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case string:
		return strconv.ParseFloat(val, 64)
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}

// calculateIntegerLevels 计算整数关口
func calculateIntegerLevels(currentPrice float64) []float64 {
	levels := []float64{}
	
	// 根据价格范围计算整数关口
	if currentPrice >= 1000 {
		// 大额币种（如 BTC）：1000, 5000, 10000, 50000, 100000 等
		base := 1000.0
		for base <= currentPrice*2 {
			if base >= currentPrice*0.5 && base <= currentPrice*2 {
				levels = append(levels, base)
			}
			if base < 10000 {
				base += 1000
			} else if base < 100000 {
				base += 10000
			} else {
				base += 50000
			}
		}
	} else if currentPrice >= 100 {
		// 中额币种（如 ETH）：100, 200, 500, 1000 等
		base := 100.0
		for base <= currentPrice*2 {
			if base >= currentPrice*0.5 && base <= currentPrice*2 {
				levels = append(levels, base)
			}
			if base < 1000 {
				base += 100
			} else {
				base += 500
			}
		}
	} else if currentPrice >= 10 {
		// 小额币种：10, 20, 50, 100 等
		base := 10.0
		for base <= currentPrice*2 {
			if base >= currentPrice*0.5 && base <= currentPrice*2 {
				levels = append(levels, base)
			}
			if base < 100 {
				base += 10
			} else {
				base += 50
			}
		}
	} else if currentPrice >= 1 {
		// 更小币种：1, 2, 5, 10 等
		base := 1.0
		for base <= currentPrice*2 {
			if base >= currentPrice*0.5 && base <= currentPrice*2 {
				levels = append(levels, base)
			}
			if base < 10 {
				base += 1
			} else {
				base += 5
			}
		}
	} else if currentPrice >= 0.1 {
		// 极小币种（0.1-1.0）：0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0 等
		minPrice := currentPrice * 0.5
		maxPrice := currentPrice * 2
		// 从0.1开始，每次增加0.1，直到超过maxPrice
		for base := 0.1; base <= maxPrice; base += 0.1 {
			if base >= minPrice && base <= maxPrice {
				levels = append(levels, base)
			}
		}
		// 也添加0.5和1.0（如果还没添加）
		if 0.5 >= minPrice && 0.5 <= maxPrice {
			// 检查是否已存在
			exists := false
			for _, l := range levels {
				if math.Abs(l-0.5) < 0.01 {
					exists = true
					break
				}
			}
			if !exists {
				levels = append(levels, 0.5)
			}
		}
		if 1.0 >= minPrice && 1.0 <= maxPrice {
			exists := false
			for _, l := range levels {
				if math.Abs(l-1.0) < 0.01 {
					exists = true
					break
				}
			}
			if !exists {
				levels = append(levels, 1.0)
			}
		}
	} else {
		// 超小币种（<0.1）：0.01, 0.02, 0.05, 0.1 等
		minPrice := currentPrice * 0.5
		maxPrice := currentPrice * 2
		// 从0.01开始，每次增加0.01，直到超过maxPrice
		for base := 0.01; base <= maxPrice; base += 0.01 {
			if base >= minPrice && base <= maxPrice {
				levels = append(levels, base)
			}
		}
		// 也添加0.05和0.1（如果还没添加）
		if 0.05 >= minPrice && 0.05 <= maxPrice {
			exists := false
			for _, l := range levels {
				if math.Abs(l-0.05) < 0.001 {
					exists = true
					break
				}
			}
			if !exists {
				levels = append(levels, 0.05)
			}
		}
		if 0.1 >= minPrice && 0.1 <= maxPrice {
			exists := false
			for _, l := range levels {
				if math.Abs(l-0.1) < 0.001 {
					exists = true
					break
				}
			}
			if !exists {
				levels = append(levels, 0.1)
			}
		}
	}
	
	// 去重并排序
	if len(levels) > 0 {
		// 使用map去重
		uniqueLevels := make(map[float64]bool)
		for _, level := range levels {
			// 四舍五入到合理精度以避免浮点数误差
			if level >= 1 {
				level = math.Round(level)
			} else if level >= 0.1 {
				level = math.Round(level*10) / 10
			} else {
				level = math.Round(level*100) / 100
			}
			uniqueLevels[level] = true
		}
		
		// 转换回数组并排序
		levels = make([]float64, 0, len(uniqueLevels))
		for level := range uniqueLevels {
			levels = append(levels, level)
		}
		
		// 按距离当前价格排序
		for i := 0; i < len(levels)-1; i++ {
			for j := i + 1; j < len(levels); j++ {
				distI := math.Abs(currentPrice - levels[i])
				distJ := math.Abs(currentPrice - levels[j])
				if distJ < distI {
					levels[i], levels[j] = levels[j], levels[i]
				}
			}
		}
		
		// 限制数量，只返回最近的5个
		if len(levels) > 5 {
			levels = levels[:5]
		}
	}
	
	return levels
}
