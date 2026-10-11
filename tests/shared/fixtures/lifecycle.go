package fixtures

import (
	"bytes"
	"crypto/sha256"
	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"strconv"
)

func LifecycleMethods() []string {
	return []string{authv1.AuthService_RefreshSession_FullMethodName, authv1.AuthService_Logout_FullMethodName, authv1.AuthService_ChangePassword_FullMethodName, sessionv1.SessionService_ListMySessions_FullMethodName, sessionv1.SessionService_RevokeMySession_FullMethodName, sessionv1.SessionService_RevokeAllMySessions_FullMethodName, sessionv1.SessionService_ListUserSessions_FullMethodName, sessionv1.SessionService_RevokeUserSession_FullMethodName, sessionv1.SessionService_RevokeAllUserSessions_FullMethodName}
}

func LifecycleEntropy() *bytes.Reader {
	data := make([]byte, 0, 16384)
	for n := 0; n < 512; n++ {
		block := sha256.Sum256([]byte("fixture-lifecycle-block-" + strconv.Itoa(n)))
		data = append(data, block[:]...)
	}
	return bytes.NewReader(data)
}
