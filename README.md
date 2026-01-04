# Minewire Server

Прокси-сервер Minewire, маскирующийся под сервер Minecraft для обхода ограничений и создания защищенных туннелей.

## Возможности

- **Шифрование AES-GCM** - весь трафик шифруется с использованием пароля клиента
- **Маскировка под Minecraft** - сервер выглядит как обычный Minecraft сервер при сканировании
- **Мультиплексирование** - множественные соединения через один туннель (yamux)
- **Симуляция игроков** - реалистичная имитация количества игроков онлайн
- **Аутентификация по паролю** - поддержка нескольких пользователей с разными паролями

## Требования

- Linux сервер (Ubuntu, Debian и т.д.)
- Go 1.19 или новее (для компиляции)
- Права root (для установки)
- Открытый порт (по умолчанию 25565)

## Установка

### Быстрая установка

```bash
cd server
sudo bash setup.sh
```

Скрипт установки автоматически:
1. Проверит наличие Go компилятора
2. Скомпилирует сервер
3. Создаст системного пользователя `minewire`
4. Установит бинарный файл в `/usr/local/bin/minewire-server`
5. Создаст конфигурацию в `/etc/minewire/server.yaml`
6. Установит systemd сервис
7. Перезагрузит systemd

### Ручная установка

Если вы предпочитаете установку вручную:

```bash
# Компиляция
cd minewire
go build -o minewire-server

# Создание пользователя
sudo useradd --system --no-create-home --shell /bin/false minewire

# Установка бинарного файла
sudo install -m 755 minewire-server /usr/local/bin/minewire-server

# Создание конфигурации
sudo mkdir -p /etc/minewire
sudo cp server.yaml /etc/minewire/server.yaml
sudo cp server-icon.png /etc/minewire/server-icon.png
sudo chown -R minewire:minewire /etc/minewire
sudo chmod 750 /etc/minewire
sudo chmod 640 /etc/minewire/server.yaml

# Установка сервиса
sudo cp minewire-server.service /etc/systemd/system/
sudo systemctl daemon-reload
```

## Настройка

### 1. Редактирование конфигурации

```bash
sudo nano /etc/minewire/server.yaml
```

### 2. Генерация безопасных паролей

Используйте OpenSSL для генерации случайных паролей:

```bash
openssl rand -hex 16
```

Пример вывода: `3d7e8a190604e9da51a3543a23421d20`

### 3. Основные параметры конфигурации

```yaml
# Порт для прослушивания (стандартный порт Minecraft)
listen_port: "25565"

# Список разрешенных паролей (замените примеры на свои!)
passwords:
  - "ВАШ_ПАРОЛЬ_1_ЗДЕСЬ"
  - "ВАШ_ПАРОЛЬ_2_ЗДЕСЬ"

# Метаданные Minecraft сервера (для маскировки)
version_name: "1.21.10"
protocol_id: 773
icon_path: "server-icon.png"
motd: "§bMinewire Proxy Server\\n§eSecure Tunnel Active"

# Настройки симуляции игроков
max_players: 20
online_min: 4
online_max: 20
```

### 4. Настройка иконки (опционально)

Замените `server-icon.png` на свою иконку (64x64 пикселя, PNG формат):

```bash
sudo cp your-icon.png /etc/minewire/server-icon.png
sudo chown minewire:minewire /etc/minewire/server-icon.png
```

## Управление сервисом

### Запуск сервера

```bash
sudo systemctl start minewire-server
```

### Остановка сервера

```bash
sudo systemctl stop minewire-server
```

### Перезапуск сервера

```bash
sudo systemctl restart minewire-server
```

### Проверка статуса

```bash
sudo systemctl status minewire-server
```

### Просмотр логов

```bash
# Просмотр последних логов
sudo journalctl -u minewire-server -n 50

# Просмотр логов в реальном времени
sudo journalctl -u minewire-server -f
```

### Автозапуск при загрузке системы

```bash
# Включить автозапуск
sudo systemctl enable minewire-server

# Отключить автозапуск
sudo systemctl disable minewire-server
```

### Настройка файрвола (UFW)

```bash
# Разрешить порт Minewire
sudo ufw allow 25565/tcp

# Включить файрвол
sudo ufw enable
```

### Настройка файрвола (firewalld)

```bash
# Разрешить порт Minewire
sudo firewall-cmd --permanent --add-port=25565/tcp
sudo firewall-cmd --reload
```

## Удаление

Для полного удаления сервера:

```bash
# Остановить и отключить сервис
sudo systemctl stop minewire-server
sudo systemctl disable minewire-server

# Удалить файлы
sudo rm /usr/local/bin/minewire-server
sudo rm /etc/systemd/system/minewire-server.service
sudo rm -rf /etc/minewire

# Удалить пользователя
sudo userdel minewire

# Перезагрузить systemd
sudo systemctl daemon-reload
```

## Архитектура

### Как это работает

1. **Маскировка**: Сервер отвечает на запросы статуса Minecraft, показывая реалистичную информацию
2. **Аутентификация**: Клиент генерирует имя пользователя из хеша пароля
3. **Туннелирование**: После аутентификации устанавливается зашифрованный yamux туннель
4. **Шифрование**: Весь трафик шифруется AES-GCM и маскируется под пакеты чанков Minecraft
5. **Проксирование**: Каждый yamux стрим проксирует соединение к целевому адресу

### Компоненты

- `main.go` - точка входа, обработка соединений
- `handler.go` - логика протокола, шифрование, туннелирование
- `protocol.go` - примитивы протокола Minecraft (VarInt, String и т.д.)
- `server.yaml` - конфигурация сервера
- `minewire-server.service` - systemd сервис
- `setup.sh` - скрипт установки

## Лицензия

MIT
