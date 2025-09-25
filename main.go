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
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
)

// Config содержит конфигурацию бота
type Config struct {
	BybitAPIKey    string
	BybitAPISecret string
	TelegramToken  string
	TelegramChatID string
	MaxTradeAmount string
	Symbols        []string
	Testnet        bool
}

// OrderBook представляет стакан ордеров
type OrderBook struct {
	Symbol string
	Asks   [][]string `json:"a"`
	Bids   [][]string `json:"b"`
}

// WebSocketMessage представляет сообщение от WebSocket
type WebSocketMessage struct {
	Topic string    `json:"topic"`
	Data  OrderBook `json:"data"`
}

// TelegramMessage представляет сообщение для Telegram
type TelegramMessage struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

// BybitAPI представляет API клиент для Bybit
type BybitAPI struct {
	APIKey    string
	APISecret string
	BaseURL   string
	Testnet   bool
}

// NewBybitAPI создает новый экземпляр API клиента
func NewBybitAPI(apiKey, apiSecret string, testnet bool) *BybitAPI {
	baseURL := "https://api.bybit.com"
	if testnet {
		baseURL = "https://api-testnet.bybit.com"
	}
	return &BybitAPI{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   baseURL,
		Testnet:   testnet,
	}
}

// generateSignature создает подпись для API запроса
func (api *BybitAPI) generateSignature(timestamp, recvWindow, params string) string {
	message := timestamp + api.APIKey + recvWindow + params
	h := hmac.New(sha256.New, []byte(api.APISecret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// getAccountBalance получает баланс аккаунта
func (api *BybitAPI) getAccountBalance() (map[string]string, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	recvWindow := "5000"
	params := ""

	signature := api.generateSignature(timestamp, recvWindow, params)

	reqURL := fmt.Sprintf("%s/v5/account/wallet-balance?accountType=UNIFIED", api.BaseURL)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-BAPI-API-KEY", api.APIKey)
	req.Header.Set("X-BAPI-SIGN", signature)
	req.Header.Set("X-BAPI-SIGN-TYPE", "2")
	req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
	req.Header.Set("X-BAPI-RECV-WINDOW", recvWindow)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	balances := make(map[string]string)
	if resultList, ok := result["result"].(map[string]interface{}); ok {
		if list, ok := resultList["list"].([]interface{}); ok && len(list) > 0 {
			if account, ok := list[0].(map[string]interface{}); ok {
				if coins, ok := account["coin"].([]interface{}); ok {
					for _, coin := range coins {
						if coinData, ok := coin.(map[string]interface{}); ok {
							if coin, ok := coinData["coin"].(string); ok {
								if free, ok := coinData["free"].(string); ok {
									balances[coin] = free
								}
							}
						}
					}
				}
			}
		}
	}

	return balances, nil
}

// placeOrder размещает ордер
func (api *BybitAPI) placeOrder(symbol, side, orderType, qty, price string) error {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	recvWindow := "5000"

	params := url.Values{}
	params.Set("category", "spot")
	params.Set("symbol", symbol)
	params.Set("side", side)
	params.Set("orderType", orderType)
	params.Set("qty", qty)
	if price != "" {
		params.Set("price", price)
	}

	signature := api.generateSignature(timestamp, recvWindow, params.Encode())

	reqURL := fmt.Sprintf("%s/v5/order/create", api.BaseURL)
	reqBody := strings.NewReader(params.Encode())

	req, err := http.NewRequest("POST", reqURL, reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-BAPI-API-KEY", api.APIKey)
	req.Header.Set("X-BAPI-SIGN", signature)
	req.Header.Set("X-BAPI-SIGN-TYPE", "2")
	req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
	req.Header.Set("X-BAPI-RECV-WINDOW", recvWindow)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	log.Printf("Order response: %s", string(body))
	return nil
}

// sendTelegramMessage отправляет сообщение в Telegram
func sendTelegramMessage(token, chatID, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	
	data := TelegramMessage{
		ChatID: chatID,
		Text:   message,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// ArbitrageBot представляет основной бот
type ArbitrageBot struct {
	config   *Config
	api      *BybitAPI
	wsConn   *websocket.Conn
	balances map[string]string
}

// NewArbitrageBot создает новый экземпляр бота
func NewArbitrageBot(config *Config) *ArbitrageBot {
	return &ArbitrageBot{
		config:   config,
		api:      NewBybitAPI(config.BybitAPIKey, config.BybitAPISecret, config.Testnet),
		balances: make(map[string]string),
	}
}

// connectWebSocket подключается к WebSocket Bybit
func (bot *ArbitrageBot) connectWebSocket() error {
	wsURL := "wss://stream-testnet.bybit.com/v5/public/spot"
	if !bot.config.Testnet {
		wsURL = "wss://stream.bybit.com/v5/public/spot"
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}

	bot.wsConn = conn

	// Подписываемся на стаканы ордеров для всех символов
	for _, symbol := range bot.config.Symbols {
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

// updateBalances обновляет балансы
func (bot *ArbitrageBot) updateBalances() error {
	balances, err := bot.api.getAccountBalance()
	if err != nil {
		return err
	}
	bot.balances = balances
	return nil
}

// findArbitrageOpportunity ищет арбитражные возможности
func (bot *ArbitrageBot) findArbitrageOpportunity(orderBook *OrderBook) {
	if len(orderBook.Asks) == 0 {
		return
	}

	// Получаем лучшую цену на покупку (самую низкую в asks)
	bestAskPrice, err := decimal.NewFromString(orderBook.Asks[0][0])
	if err != nil {
		return
	}

	bestAskQty, err := decimal.NewFromString(orderBook.Asks[0][1])
	if err != nil {
		return
	}

	// Получаем лучшую цену на продажу (самую высокую в bids)
	if len(orderBook.Bids) == 0 {
		return
	}

	bestBidPrice, err := decimal.NewFromString(orderBook.Bids[0][0])
	if err != nil {
		return
	}

	// Проверяем арбитражную возможность
	// Если цена покупки значительно ниже цены продажи
	spread := bestBidPrice.Sub(bestAskPrice)
	spreadPercent := spread.Div(bestAskPrice).Mul(decimal.NewFromInt(100))

	// Минимальный спред для арбитража (0.1%)
	minSpread := decimal.NewFromFloat(0.1)
	if spreadPercent.GreaterThan(minSpread) {
		bot.executeArbitrage(orderBook.Symbol, bestAskPrice, bestAskQty, bestBidPrice)
	}
}

// executeArbitrage выполняет арбитражную сделку
func (bot *ArbitrageBot) executeArbitrage(symbol string, buyPrice, qty, sellPrice decimal.Decimal) {
	// Обновляем балансы перед сделкой
	if err := bot.updateBalances(); err != nil {
		log.Printf("Ошибка обновления балансов: %v", err)
		return
	}

	// Определяем базовую валюту (например, BTC для BTCUSDT)
	baseCurrency := strings.Replace(symbol, "USDT", "", 1)
	quoteCurrency := "USDT"

	// Проверяем доступный баланс
	availableBalance, exists := bot.balances[quoteCurrency]
	if !exists {
		log.Printf("Нет баланса для %s", quoteCurrency)
		return
	}

	availableBalanceDecimal, err := decimal.NewFromString(availableBalance)
	if err != nil {
		log.Printf("Ошибка парсинга баланса: %v", err)
		return
	}

	// Рассчитываем сумму сделки
	tradeAmount := buyPrice.Mul(qty)
	maxTradeAmount, _ := decimal.NewFromString(bot.config.MaxTradeAmount)

	// Ограничиваем размер сделки
	if tradeAmount.GreaterThan(maxTradeAmount) {
		qty = maxTradeAmount.Div(buyPrice)
		tradeAmount = maxTradeAmount
	}

	// Проверяем достаточность баланса
	if tradeAmount.GreaterThan(availableBalanceDecimal) {
		log.Printf("Недостаточно баланса для сделки. Доступно: %s, Требуется: %s", 
			availableBalance, tradeAmount.String())
		return
	}

	log.Printf("Выполняем арбитраж для %s: покупаем %s по цене %s, продаем по %s", 
		symbol, qty.String(), buyPrice.String(), sellPrice.String())

	// Размещаем ордер на покупку
	if err := bot.api.placeOrder(symbol, "Buy", "Limit", qty.String(), buyPrice.String()); err != nil {
		log.Printf("Ошибка размещения ордера на покупку: %v", err)
		return
	}

	// Ждем немного перед размещением ордера на продажу
	time.Sleep(1 * time.Second)

	// Размещаем ордер на продажу по рыночной цене
	if err := bot.api.placeOrder(symbol, "Sell", "Market", qty.String(), ""); err != nil {
		log.Printf("Ошибка размещения ордера на продажу: %v", err)
		return
	}

	// Рассчитываем прибыль
	profit := sellPrice.Sub(buyPrice).Mul(qty)
	profitPercent := profit.Div(tradeAmount).Mul(decimal.NewFromInt(100))

	// Отправляем уведомление в Telegram
	message := fmt.Sprintf("🎯 Арбитраж выполнен!\n"+
		"Символ: %s\n"+
		"Количество: %s\n"+
		"Цена покупки: %s\n"+
		"Цена продажи: %s\n"+
		"Прибыль: %s USDT (%.2f%%)\n"+
		"Время: %s",
		symbol, qty.String(), buyPrice.String(), sellPrice.String(),
		profit.String(), profitPercent.InexactFloat64(),
		time.Now().Format("2006-01-02 15:04:05"))

	if err := sendTelegramMessage(bot.config.TelegramToken, bot.config.TelegramChatID, message); err != nil {
		log.Printf("Ошибка отправки сообщения в Telegram: %v", err)
	}
}

// run запускает бота
func (bot *ArbitrageBot) run() error {
	// Подключаемся к WebSocket
	if err := bot.connectWebSocket(); err != nil {
		return fmt.Errorf("ошибка подключения к WebSocket: %v", err)
	}
	defer bot.wsConn.Close()

	// Обновляем балансы
	if err := bot.updateBalances(); err != nil {
		return fmt.Errorf("ошибка получения балансов: %v", err)
	}

	log.Println("Бот запущен и ожидает арбитражных возможностей...")

	// Основной цикл
	for {
		var msg WebSocketMessage
		if err := bot.wsConn.ReadJSON(&msg); err != nil {
			log.Printf("Ошибка чтения WebSocket сообщения: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if msg.Topic != "" && strings.Contains(msg.Topic, "orderbook") {
			bot.findArbitrageOpportunity(&msg.Data)
		}
	}
}

func main() {
	// Загружаем переменные окружения
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используем переменные окружения системы")
	}

	config := &Config{
		BybitAPIKey:    os.Getenv("BYBIT_API_KEY"),
		BybitAPISecret: os.Getenv("BYBIT_API_SECRET"),
		TelegramToken:  os.Getenv("TELEGRAM_TOKEN"),
		TelegramChatID: os.Getenv("TELEGRAM_CHAT_ID"),
		MaxTradeAmount: os.Getenv("MAX_TRADE_AMOUNT"),
		Testnet:        os.Getenv("BYBIT_TESTNET") == "true",
	}

	// Парсим символы из переменной окружения
	symbolsStr := os.Getenv("TRADING_SYMBOLS")
	if symbolsStr != "" {
		config.Symbols = strings.Split(symbolsStr, ",")
	} else {
		config.Symbols = []string{"BTCUSDT", "ETHUSDT", "ADAUSDT"}
	}

	// Проверяем обязательные параметры
	if config.BybitAPIKey == "" || config.BybitAPISecret == "" {
		log.Fatal("Необходимо указать BYBIT_API_KEY и BYBIT_API_SECRET")
	}

	if config.TelegramToken == "" || config.TelegramChatID == "" {
		log.Fatal("Необходимо указать TELEGRAM_TOKEN и TELEGRAM_CHAT_ID")
	}

	if config.MaxTradeAmount == "" {
		config.MaxTradeAmount = "10"
	}

	log.Printf("Конфигурация: Testnet=%v, Символы=%v, Макс. сумма сделки=%s", 
		config.Testnet, config.Symbols, config.MaxTradeAmount)

	// Создаем и запускаем бота
	bot := NewArbitrageBot(config)
	if err := bot.run(); err != nil {
		log.Fatal(err)
	}
}