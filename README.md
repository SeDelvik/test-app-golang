# Предисловие
Для базы данных использовалась PostgreSQL 12. docker-compose и выноса подключения в env-параметры нет.
# Порядок установки
 - Добавить в dbKey.txt корректные данные для подключения к базе данных
 - из папки приложения через консоль прописать:
```sh
go run .
```
 - порт подключения 8181