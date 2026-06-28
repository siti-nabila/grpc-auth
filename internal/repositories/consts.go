package repositories

type (
	DbSource  string
	TableName string
)

const (
	UserDbSource DbSource = "user"

	// Table Names
	AuthTable    TableName = "auth"
	ProfileTable TableName = "profile"
)
