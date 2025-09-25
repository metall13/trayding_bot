package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
)

type Config struct {
	BybitAPIKey    string
	BybitAPISecret string
	TelegramToken  string
	TelegramChatID string
	MaxTradeAmount string
	Testnet        bool
}

type OrderBook struct {
	Symbol string `json:"s"`
	Asks   [][]string `json:"a"`
	Bids   [][]string `json:"b"`
}

type OrderBookResponse struct {
	Topic string    `json:"topic"`
	Data  OrderBook `json:"data"`
}

type PlaceOrderRequest struct {
	Symbol   string `json:"symbol"`
	Side     string `json:"side"`
	OrderType string `json:"orderType"`
	Qty      string `json:"qty"`
	Price    string `json:"price,omitempty"`
}

type PlaceOrderResponse struct {
	RetCode int `json:"retCode"`
	Result  struct {
		OrderID string `json:"orderId"`
	} `json:"result"`
}

type AccountBalance struct {
	RetCode int `json:"retCode"`
	Result  struct {
		List []struct {
			Coin     string `json:"coin"`
			Free     string `json:"free"`
			Locked   string `json:"locked"`
		} `json:"list"`
	} `json:"result"`
}

type TelegramMessage struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

type ArbitrageBot struct {
	config     *Config
	wsConn     *websocket.Conn
	httpClient *http.Client
	baseURL    string
}

func NewArbitrageBot() (*ArbitrageBot, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используем переменные окружения")
	}

	config := &Config{
		BybitAPIKey:    os.Getenv("BYBIT_API_KEY"),
		BybitAPISecret: os.Getenv("BYBIT_API_SECRET"),
		TelegramToken:  os.Getenv("TELEGRAM_TOKEN"),
		TelegramChatID: os.Getenv("TELEGRAM_CHAT_ID"),
		MaxTradeAmount: os.Getenv("MAX_TRADE_AMOUNT"),
		Testnet:        os.Getenv("TESTNET") == "true",
	}

	if config.BybitAPIKey == "" || config.BybitAPISecret == "" {
		return nil, fmt.Errorf("необходимо указать BYBIT_API_KEY и BYBIT_API_SECRET")
	}

	if config.TelegramToken == "" || config.TelegramChatID == "" {
		return nil, fmt.Errorf("необходимо указать TELEGRAM_TOKEN и TELEGRAM_CHAT_ID")
	}

	if config.MaxTradeAmount == "" {
		config.MaxTradeAmount = "10" // По умолчанию 10 USDT
	}

	baseURL := "https://api.bybit.com"
	if config.Testnet {
		baseURL = "https://api-testnet.bybit.com"
	}

	return &ArbitrageBot{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}, nil
}

func (bot *ArbitrageBot) generateSignature(timestamp, method, endpoint, body string) string {
	payload := timestamp + bot.config.BybitAPIKey + "5000" + body
	h := hmac.New(sha256.New, []byte(bot.config.BybitAPISecret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

func (bot *ArbitrageBot) makeAuthenticatedRequest(method, endpoint string, body interface{}) (*http.Response, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	
	var bodyStr string
	if body != nil {
		bodyBytes, _ := json.Marshal(body)
		bodyStr = string(bodyBytes)
	}

	signature := bot.generateSignature(timestamp, method, endpoint, bodyStr)

	req, err := http.NewRequest(method, bot.baseURL+endpoint, bytes.NewBufferString(bodyStr))
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-BAPI-API-KEY", bot.config.BybitAPIKey)
	req.Header.Set("X-BAPI-SIGN", signature)
	req.Header.Set("X-BAPI-SIGN-TYPE", "2")
	req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
	req.Header.Set("X-BAPI-RECV-WINDOW", "5000")
	req.Header.Set("Content-Type", "application/json")

	return bot.httpClient.Do(req)
}

func (bot *ArbitrageBot) getAccountBalance() (*AccountBalance, error) {
	resp, err := bot.makeAuthenticatedRequest("GET", "/v5/account/wallet-balance", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var balance AccountBalance
	if err := json.Unmarshal(body, &balance); err != nil {
		return nil, err
	}

	return &balance, nil
}

func (bot *ArbitrageBot) placeOrder(symbol, side, orderType, qty, price string) (*PlaceOrderResponse, error) {
	orderReq := PlaceOrderRequest{
		Symbol:   symbol,
		Side:     side,
		OrderType: orderType,
		Qty:      qty,
	}

	if orderType == "Limit" && price != "" {
		orderReq.Price = price
	}

	resp, err := bot.makeAuthenticatedRequest("POST", "/v5/order/create", orderReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var orderResp PlaceOrderResponse
	if err := json.Unmarshal(body, &orderResp); err != nil {
		return nil, err
	}

	return &orderResp, nil
}

func (bot *ArbitrageBot) sendTelegramMessage(message string) error {
	telegramURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", bot.config.TelegramToken)
	
	msg := TelegramMessage{
		ChatID: bot.config.TelegramChatID,
		Text:   message,
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	resp, err := http.Post(telegramURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (bot *ArbitrageBot) connectWebSocket() error {
	wsURL := "wss://stream.bybit.com/v5/public/spot"
	if bot.config.Testnet {
		wsURL = "wss://stream-testnet.bybit.com/v5/public/spot"
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}

	bot.wsConn = conn

	// Подписываемся на стакан ордеров для основных пар
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "ADAUSDT", "SOLUSDT"}
	
	for _, symbol := range symbols {
		subscribeMsg := map[string]interface{}{
			"op":   "subscribe",
			"args": []string{fmt.Sprintf("orderbook.1.%s", symbol)},
		}

		if err := conn.WriteJSON(subscribeMsg); err != nil {
			return err
		}
	}

	return nil
}

func (bot *ArbitrageBot) processOrderBook(data OrderBook) {
	if len(data.Asks) == 0 || len(data.Bids) == 0 {
		return
	}

	// Получаем лучшую цену на покупку (bid) и продажу (ask)
	bestBidPrice, _ := decimal.NewFromString(data.Bids[0][0])
	bestAskPrice, _ := decimal.NewFromString(data.Asks[0][0])

	// Ищем арбитражные возможности
	for _, ask := range data.Asks {
		askPrice, err := decimal.NewFromString(ask[0])
		if err != nil {
			continue
		}

		askQty, err := decimal.NewFromString(ask[1])
		if err != nil {
			continue
		}

		// Если цена ask ниже лучшей bid цены, есть арбитражная возможность
		if askPrice.LessThan(bestBidPrice) {
			bot.executeArbitrage(data.Symbol, askPrice, askQty, bestBidPrice)
		}
	}
}

func (bot *ArbitrageBot) executeArbitrage(symbol string, buyPrice, qty, sellPrice decimal.Decimal) {
	// Проверяем лимит на сделку
	maxAmount, _ := decimal.NewFromString(bot.config.MaxTradeAmount)
	tradeAmount := buyPrice.Mul(qty)

	if tradeAmount.GreaterThan(maxAmount) {
		// Уменьшаем количество до максимального лимита
		qty = maxAmount.Div(buyPrice)
		tradeAmount = maxAmount
	}

	// Проверяем баланс
	balance, err := bot.getAccountBalance()
	if err != nil {
		log.Printf("Ошибка получения баланса: %v", err)
		return
	}

	var usdtBalance decimal.Decimal
	for _, coin := range balance.Result.List {
		if coin.Coin == "USDT" {
			usdtBalance, _ = decimal.NewFromString(coin.Free)
			break
		}
	}

	if usdtBalance.LessThan(tradeAmount) {
		log.Printf("Недостаточно USDT для арбитража. Доступно: %s, нужно: %s", usdtBalance.String(), tradeAmount.String())
		return
	}

	log.Printf("Найдена арбитражная возможность: %s", symbol)
	log.Printf("Покупка по цене: %s, Продажа по цене: %s", buyPrice.String(), sellPrice.String())

	// Размещаем лимитный ордер на покупку
	buyOrder, err := bot.placeOrder(symbol, "Buy", "Limit", qty.String(), buyPrice.String())
	if err != nil || buyOrder.RetCode != 0 {
		log.Printf("Ошибка размещения ордера на покупку: %v", err)
		return
	}

	log.Printf("Ордер на покупку размещен: %s", buyOrder.Result.OrderID)

	// Ждем исполнения ордера (в реальном боте нужно проверять статус)
	time.Sleep(2 * time.Second)

	// Размещаем рыночный ордер на продажу
	sellOrder, err := bot.placeOrder(symbol, "Sell", "Market", qty.String(), "")
	if err != nil || sellOrder.RetCode != 0 {
		log.Printf("Ошибка размещения ордера на продажу: %v", err)
		return
	}

	log.Printf("Ордер на продажу размещен: %s", sellOrder.Result.OrderID)

	// Рассчитываем прибыль
	profit := sellPrice.Sub(buyPrice).Mul(qty)
	profitPercent := profit.Div(tradeAmount).Mul(decimal.NewFromInt(100))

	// Отправляем уведомление в Telegram
	message := fmt.Sprintf(
		"🎯 Арбитраж выполнен!\n"+
		"Пара: %s\n"+
		"Покупка: %s USDT по цене %s\n"+
		"Продажа: %s USDT по цене %s\n"+
		"Прибыль: %s USDT (%.2f%%)\n"+
		"Время: %s",
		symbol,
		qty.String(), buyPrice.String(),
		qty.String(), sellPrice.String(),
		profit.String(), profitPercent.InexactFloat64(),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if err := bot.sendTelegramMessage(message); err != nil {
		log.Printf("Ошибка отправки сообщения в Telegram: %v", err)
	}
}

func (bot *ArbitrageBot) run() error {
	if err := bot.connectWebSocket(); err != nil {
		return fmt.Errorf("ошибка подключения к WebSocket: %v", err)
	}
	defer bot.wsConn.Close()

	log.Println("Бот запущен и подключен к Bybit WebSocket")

	// Отправляем стартовое сообщение
	bot.sendTelegramMessage("🤖 Арбитражный бот запущен и готов к работе!")

	for {
		var response OrderBookResponse
		if err := bot.wsConn.ReadJSON(&response); err != nil {
			log.Printf("Ошибка чтения WebSocket: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if response.Topic != "" && response.Data.Symbol != "" {
			bot.processOrderBook(response.Data)
		}
	}
}

func main() {
	bot, err := NewArbitrageBot()
	if err != nil {
		log.Fatalf("Ошибка создания бота: %v", err)
	}

	log.Println("Запуск арбитражного бота...")
	if err := bot.run(); err != nil {
		log.Fatalf("Ошибка работы бота: %v", err)
	}
}