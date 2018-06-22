# ongrid-thrift

## Ongrid thrift service

Точка входа в программу - функция `main()` из модуля main.go. Из неё вызывается функция `runServer()` из модуля server.go, которая создает обработчики thrift протокола. Все вызовы методов thrift обрабатываются в модуле handler.go.

### Modules

#### main.go

`main()`, здесь запускаеться сервер thrift протокола.

#### server.go

`runServer()`, здесь создаются обработчики thrift сервисов DB и Ongrid.

#### db_struct.go

В модуле описываются структуры (DBAuth, DBRequest, DBClient, DBCar, DBPerson, DBCompany, DBEvent) для загрузки данных из БД. И функции загрузки этих данных из БД.

#### handler.go

Модуль с реализацией всех методов thrift сервисов DB и Ongrid. В thrift методах authToken, token - токен авторизации. Он возвращяется в методах `Connect()` и `AddWorkPlace()`, а генерируется в функции `startSession()`. Остальные методы используют этот токен для проверки авторизации пользователя и выбора текущей сессии. Для выхода из сессии используется метод `Disconnect()`. Сессия это структура вида: 

```
type Session struct {
	token         string
	user          *User
	queries       map[string][]ongrid2.Query
	transactionID int
	db            *sqlx.DB
	config        *ongrid2.ConfigObject
}
```
где 
- token - токен авторизации,
- user - текущий клиент,
- queries - список запросов для работы с транзакциями,
- transactionID - id текущей транзакции,
- db - текущяя БД пользователя (firebird),
- config - конфигурация (из igo$objects)

Сессии хранятся в хеше sessions, ключем является id сессии.

`init()` - считывает настройки системной БД из файла ongrid.conf в структуру.

`Ongrid.Connect(macAddr string) (token string, err error)` - авторизация в системе по мак адресу. Входящие параметры: macAddr - мак адрес. Исходящие параметры: token - токен авторизации.

`Ongrid.AddWorkPlace(wpName, macAddr, login, password string) (token string, err error)` - добовляет новое рабочее место в БД. Входящие параметры: wpName - имя рабочего места, macAddr - мак адрес, login - логин, password - пароль. Исходящие параметры: token - токен авторизации.

`Ongrid.Disconnect(authToken string)` - выход из системы. Входящие параметры: authToken - токен авторизации.

`DB.ExecuteSelectQuery(authToken string, query *ongrid2.Query) (*ongrid2.DataRowSet, error)` - выполняет sql запрос и возвращает результат. Входящие параметры: authToken - токен авторизации, query - sql запрос. Исходящие параметры: DataRowSet - dataset с ответом.

`DB.ExecuteNonSelectQuery(authToken string, query *ongrid2.Query)` - выполныет sql запрос без возврата результата (update, insert, delete).  Входящие параметры: authToken - токен авторизации, query - sql запрос.

`DB.StartBatchExecution(authToken string) (string, error)` - аналог StartTransaction. Входящие параметры: authToken - токен авторизации. Исходящие параметры: возвращает строчку с id транзакции.

`DB.AddQuery(authToken string, batchID string, query *ongrid2.Query)` - добавляет sql запрос к транзацкии. Входящие параметры: authToken - токен авторизации, batchID - id транзакции, query - sql запрос.

`DB.FinishBatchExecution(authToken string, batchID string, condition *ongrid2.Query, onSuccess *ongrid2.Query)` - выполняет все добавленные запросы и завершает транзакцию. Входящие параметры: authToken - токен авторизации, batchID - id транзакции, condition - не используется, onSuccess - sql запрос, кторый выполныется в случае удачной завершении транзакции.

`DB.BatchExecute(authToken string, queries []*ongrid2.Query, condition *ongrid2.Query, onSuccess *ongrid2.Query)` - выполныет список sql запросов в одной транзакции. Входящие параметры: authToken - токен авторизации, queries - список sql запросов, condition - не используется, onSuccess - sql запрос, кторый выполныется в случае удачной завершении транзакции.

`Ongrid.GetEvents(authToken, last string) (events []*ongrid2.Event, err error)` - возвращает последние эвенеты, Входящие параметры: authToken - токен авторизации, last - id последнего эвента. Исходящие параметры: events - список эвентов.

`Ongrid.PostEvent(authToken string, event *ongrid2.Event) (string, error)` - создание новго эвента. Входящие параметры: authToken - токен авторизации, event - новый эвент. Исходящие параметры: id новго эвента.

`Ongrid.GetCentrifugoConf(authToken string) (*ongrid2.CentrifugoConf, error)` - запрос параметров сервиса Centrufugo. Входящие параметры: authToken - токен авторизации. Исходящие параметры: настройки сервиса.

`Ongrid.GetConfiguration(authToken string, userID int64) (*ongrid2.ConfigObject, error)` - запрос конфигурации системы из таблицы igo$objects и сохранение конфигурации в текущей сессии. Входящие параметры: authToken - токен авторизации, userID - id пользователя. Исходящие параметры: конфигурация.

`Ongrid.GetProps(authToken string) (props []*ongrid2.ConfigProp, err error)` - запрос свойств из таблицы igo$props. Входящие параметры: authToken - токен авторизации. Исходящие параметры: список свойств.

`Ongrid.GetPermissions(authToken string, userID int64) ([]*ongrid2.ConfigPermission, error)` - не используется.

`Ongrid.SetPermission(authToken string, permission *ongrid2.ConfigPermission) error ` - не используется.

`Ongrid.GetUserPrivileges(authToken string, userID int64) ([]*ongrid2.Privilege, error)` - запрос прав пользователя. Входящие параметры: authToken - токен авторизации, userID - id пользователя, для которого запрашивается конфигурация. Исходящие параметры: список разрешений для данного пользователя. Здесь используется метод aclService.GetACL из модуля privileges.

##### Вспомогательные функции

`getParams(query *ongrid2.Query) map[string]interface{}` - Возвращает все параметры из объекта query, sql запроса.

`authMac(macAddr string) (string, error)` - аутентификация по мак адресу, создает сессию пользователя вызовом `startSession()`.

`authLP(login, password string) (string, *User, error)` - аутентификация по логину и паролю, создает сессию пользователя вызовом `startSession()`.

`startSession(user *User) (authToken string, err error)` - создает сессию, генерирует токен авторизации для сервисов. Входящие параметры: user - авторизованный пользователь. Исходящие параметры: токен авторизации.

`checkToken(authToken string) (string, error)` - проверяет токен и возвращает id сессии в случае успеха.

`getSessionIDByToken(token string) (string, error)` - вспомогательная функция поиска токена в массиве сессий.

#### mongo.go

Модуль для работы с MongoDB

`NewMongoConnection(mgoConfig MongoConfig) (conn *MongoConnection)` - создает новое соединение с базой. Входящие параметры: конфигурация MongoDB. Исходящие параметры: коннект с базой.

`createLocalConnection(mgoConfig MongoConfig)` - подключается к базе и конкретной коллекции в mongo. Вызывается из `NewMongoConnection()`.

`CloseConnection()` - закрытие соединения с mongo.

`GetUserByMacAddr(macAddr string) (user User, err error)` - запрашивает клиента из коллекции clients по мак адресу. Входящие параметры: macAddr - мак адрес. Исходящие параметры: user - клиент.

`GetUserByLogin(login string) (user User, err error)` - запрашивает клиента из коллекции clients по логину. Входящие параметры: логин. Исходящие параметры: user - клиент.

`ClientAddWorkPlace(id string, wpName string, macAddr string) error` - добавляен новое рабочее место в БД в коллекцию clients. Входящие параметры: id - id клента, wpName - имя рабочего места, macAddr - мак адрес.

