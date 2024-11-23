package models

import "fmt"

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

//JSON struct that holds generated authorization token
type AuthToken struct {
	Token string `json:"token"`
}

func (s AuthToken) String() string {
	return fmt.Sprintf("%v", s.Token)
}
