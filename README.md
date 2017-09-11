# ongrid-thrift
Ongrid thrift service 

string login(1: string login, 2: string password) throws (1: UserException userException),
void logout(1: string authToken),
DataRowSet executeSelectQuery(1: string authToken, 2: Query query) throws (1: IntergridException intergridException, 2: UserException userException),
void executeNonSelectQuery(1: string authToken, 2: Query query) throws (1: IntergridException intergridException, 2: UserException userException),
string startBatchExecution(1: string authToken) throws (1: IntergridException intergridException, 2: UserException userException),
void addQuery(1: string authToken, 2: string batchID, 3: Query query) throws (1: IntergridException intergridException, 2: UserException userException),
string finishBatchExecution(1: string authToken, 2: string batchID, 3: Query condition, 4: Query onSuccess) throws (1: IntergridException intergridException, 2: UserException userException),
string batchExecute(1: string authToken, 2: list<Query> queries, 3: Query condition, 4: Query onSuccess) throws (1: IntergridException intergridException, 2: UserException userException),
