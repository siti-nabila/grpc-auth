package domain

// ProfileUpdate separates input presence from the persistence DTO. Ownership is
// always taken from the authenticated principal, never supplied by this input.
type ProfileUpdate struct {
	Name, Address, Phone string
	CountryCode          *string
	Mask                 []string
	MaskPresent          bool
}

type UserVersionWriter interface {
	SetVersion(userID uint64, version int64) error
}

type UserVersionUpdate struct {
	ID      uint64 `sql:"column:id;primaryKey"`
	Version int64  `sql:"column:version"`
}

func (UserVersionUpdate) TableName() string { return "auth" }
