package repo

import "gorm.io/gorm"

type Repo struct {
	db *gorm.DB
}

func NewSessionRepo(db *gorm.DB) *Repo {
	return &Repo{
		db: db,
	}
}

type GeneralPurposeAPI interface{}
