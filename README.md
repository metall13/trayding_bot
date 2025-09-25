# Bybit Arbitrage Bot

Простой торговый бот на Golang для арбитража на бирже Bybit.

## Описание

Бот подключается к WebSocket Bybit, получает стакан ордеров и ищет арбитражные возможности. При обнаружении выгодной сделки бот автоматически покупает по низкой цене и продает по высокой, отправляя уведомления в Telegram.

## Функции

- ✅ Подключение к WebSocket Bybit для получения стакана ордеров
- ✅ Автоматический поиск арбитражных возможностей
- ✅ Выполнение арбитражных сделок
- ✅ Уведомления в Telegram о каждой сделке
- ✅ Риск-менеджмент с лимитами на размер сделки
- ✅ Работа на testnet Bybit
- ✅ Контейнеризация с Docker

## Требования

- Docker и Docker Compose
- API ключи от Bybit
- Telegram бот токен

## Установка и запуск

### 1. Клонирование репозитория

```bash
git clone <repository-url>
cd bybit-arbitrage-bot
```

### 2. Настройка переменных окружения

Скопируйте файл `.env.example` в `.env` и заполните необходимые параметры:

```bash
cp .env.example .env
```

Отредактируйте `.env` файл:

```env
# Bybit API настройки
BYBIT_API_KEY=ybY7KIlrM9uV8cxi6i
BYBIT_API_SECRET=m9EMyZoYfOKThyeope1WFVfAy15PSDUFdrBa

# Telegram настройки
TELEGRAM_TOKEN=1719992670:AAFC7vPnPDgbdgs2DK4CxQANwORcSoNJ3sM
TELEGRAM_CHAT_ID=137767262

# Торговые настройки
MAX_TRADE_AMOUNT=10
TRADING_SYMBOLS=BTCUSDT,ETHUSDT,ADAUSDT

# Использовать testnet (true/false)
BYBIT_TESTNET=true
```

### 3. Получение API ключей Bybit

1. Зарегистрируйтесь на [Bybit](https://www.bybit.com/)
2. Перейдите в раздел API Management
3. Создайте новый API ключ с правами на торговлю
4. **ВАЖНО**: Используйте testnet для тестирования!

### 4. Настройка Telegram бота

1. Создайте бота через [@BotFather](https://t.me/botfather)
2. Получите токен бота
3. Узнайте свой Chat ID (можно использовать [@userinfobot](https://t.me/userinfobot))

### 5. Запуск бота

```bash
# Сборка и запуск через Docker Compose
docker-compose up --build

# Или запуск в фоновом режиме
docker-compose up -d --build
```

### 6. Просмотр логов

```bash
# Просмотр логов в реальном времени
docker-compose logs -f

# Просмотр логов конкретного сервиса
docker-compose logs -f bybit-arbitrage-bot
```

## Конфигурация

### Переменные окружения

| Переменная | Описание | Пример |
|------------|----------|---------|
| `BYBIT_API_KEY` | API ключ от Bybit | `ybY7KIlrM9uV8cxi6i` |
| `BYBIT_API_SECRET` | API секрет от Bybit | `m9EMyZoYfOKThyeope1WFVfAy15PSDUFdrBa` |
| `TELEGRAM_TOKEN` | Токен Telegram бота | `1719992670:AAFC7vPnPDgbdgs2DK4CxQANwORcSoNJ3sM` |
| `TELEGRAM_CHAT_ID` | ID чата для уведомлений | `137767262` |
| `MAX_TRADE_AMOUNT` | Максимальная сумма сделки в USDT | `10` |
| `TRADING_SYMBOLS` | Торговые пары через запятую | `BTCUSDT,ETHUSDT,ADAUSDT` |
| `BYBIT_TESTNET` | Использовать testnet | `true` |

### Торговые пары

По умолчанию бот торгует следующими парами:
- BTCUSDT
- ETHUSDT  
- ADAUSDT

Вы можете изменить список в переменной `TRADING_SYMBOLS`.

## Логика работы

1. **Подключение**: Бот подключается к WebSocket Bybit и подписывается на стаканы ордеров
2. **Анализ**: Для каждой торговой пары анализируется разница между лучшими ценами покупки и продажи
3. **Арбитраж**: При обнаружении спреда > 0.1% выполняется арбитражная сделка:
   - Покупка по лучшей цене (лимитный ордер)
   - Продажа по рыночной цене
4. **Уведомления**: Отправка детальной информации о сделке в Telegram

## Риск-менеджмент

- ✅ Лимит на размер одной сделки
- ✅ Проверка доступного баланса перед сделкой
- ✅ Работа только на testnet для безопасности
- ✅ Минимальный спред для арбитража (0.1%)

## Мониторинг

Бот отправляет уведомления в Telegram при каждой сделке, включая:
- Символ торговой пары
- Количество и цены
- Рассчитанную прибыль
- Время выполнения сделки

## Остановка бота

```bash
# Остановка через Docker Compose
docker-compose down

# Остановка с удалением контейнеров
docker-compose down --rmi all
```

## Разработка

### Локальная разработка

```bash
# Установка зависимостей
go mod download

# Запуск локально
go run main.go
```

### Сборка без Docker

```bash
# Сборка для Linux
GOOS=linux GOARCH=amd64 go build -o bybit-arbitrage-bot main.go

# Сборка для текущей платформы
go build -o bybit-arbitrage-bot main.go
```

## Безопасность

⚠️ **ВАЖНЫЕ ПРЕДУПРЕЖДЕНИЯ**:

1. **Всегда используйте testnet** для тестирования
2. **Не храните API ключи** в открытом виде
3. **Ограничьте права API ключей** только необходимыми операциями
4. **Начните с малых сумм** для тестирования
5. **Мониторьте логи** на предмет ошибок

## Поддержка

При возникновении проблем:

1. Проверьте логи: `docker-compose logs -f`
2. Убедитесь в правильности API ключей
3. Проверьте баланс на testnet
4. Убедитесь в корректности Telegram настроек

## Лицензия

MIT License