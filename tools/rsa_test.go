package tools_test

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/air-iot/service/model"
	"github.com/air-iot/service/tools"
)

func TestSignatureAndVerifyRSA(t *testing.T) {

	// 公私钥
	privateKey := `-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQC9qv6ualIbyIvOcRKaBwBckxMDvmfMb4JSgL4LEvuNzky+QafM
JiENdiQDuhOLrjhQAmyLZBOebR91aZtEfftODglwCdqDsizCDdxwJ4lU4eMc9xFk
TcoEtkKEgS0MANL1XCDUrNdj4q5R3hP/a6vJ0j91bBWOIA/q/DNZPhMqrQIDAQAB
AoGAEPizS9eTK83E5s7a34H3YBE//xKDVrl2o5vGBZcn/78hQlf9JAkYZYw3+kZk
99d5zfz9ifaFQ+SXT0oJHPaZ7TEzOxamQAf71MRp6REYDiYtwtOcrZ8nEYQxnK9v
dzryA9IMRrmwxkUxy70Y/rrjVZJgePX+zWmjdIKl39N18XECQQDG1s+eYGpjKaTb
Ct5/DRPdXSS+sFC20TtSgcRnbYS8aKFWj3zIeyWV8evITIDWmrNMSe53QdNxLPG3
v3t1c9gRAkEA9DE/9bn1ELkyQERvcaDbwxmsgd7/ibbk287EypXXmFoZWPnfZYXL
CCQipM1zK/od2+E8J/IbExRAwf8iwPtk3QJAN2Hxpj1YpJIe1tvqKR0tYUTmTS6y
7JjOmyaF3AEHLas+9Os2aGjUiTU+5SVZ5WxlcGRPRWxSRn2sPe/ZpVdE4QJBAKs1
AJALchvojKfsk2pKiuouTPm9XNK0TZ8jSGx1RHSH7y+n+Y4XkTNDCpsbhL13nom0
UFX9dCgbUg/yDu7ZE20CQBZaO7mm4RZns64q7KksJeSEo6NgApYAqdRgKl1Y7gZr
2GJ7V2Vu/U2E0O07gxtDAUNcneBw8dnCIJ+gEWhLoUI=
-----END RSA PRIVATE KEY-----`

	publicKey := `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC9qv6ualIbyIvOcRKaBwBckxMD
vmfMb4JSgL4LEvuNzky+QafMJiENdiQDuhOLrjhQAmyLZBOebR91aZtEfftODglw
CdqDsizCDdxwJ4lU4eMc9xFkTcoEtkKEgS0MANL1XCDUrNdj4q5R3hP/a6vJ0j91
bBWOIA/q/DNZPhMqrQIDAQAB
-----END PUBLIC KEY-----`

	// 构造明文
	license := new(model.License)
	license.Validity = true
	license.Trial = true
	license.Message = "12345"

	signature := new(model.Signature)
	signature.License = license

	plainText, err := json.Marshal(signature.License)
	if err != nil {
		t.Fatal(err)
	}

	// 签名
	sigText, err := tools.SignatureRSA(plainText, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	signature.SignatureText = sigText
	log.Printf("签名:%+v", *signature)

	// 验签
	b, err := tools.VerifyRSA(plainText, signature.SignatureText, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	log.Println("验证结果:", b)
}
