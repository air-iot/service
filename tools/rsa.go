package tools

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

// RSA签名 - 私钥
func SignatureRSA(plainText []byte, privateKey string) ([]byte, error) {

	block, _ := pem.Decode([]byte(privateKey))
	// x509将数据解析成私钥结构体 -> 得到了私钥
	privateKey_, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	// sha512
	hashText := sha512.Sum512(plainText)
	sigText, err := rsa.SignPKCS1v15(rand.Reader, privateKey_, crypto.SHA512, hashText[:])
	if err != nil {
		return nil, err
	}
	return sigText, nil
}

// RSA签名验证
func VerifyRSA(plainText, sigText []byte, publicKey string) (bool, error) {

	// 使用pem解码 -> 得到pem.Block结构体变量
	block, _ := pem.Decode([]byte(publicKey))
	// 使用x509对pem.Block中的Bytes变量中的数据进行解析 ->  得到一接口
	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false, err
	}
	// 进行类型断言得到公钥结构体
	publicKey_, ok := pubInterface.(*rsa.PublicKey)
	if !ok {
		return false, errors.New("公钥格式错误！")
	}
	// 对原始消息进行哈希运算(和签名使用的哈希算法一致) -> 散列值
	hashText := sha512.Sum512(plainText)
	// 签名认证 - rsa中的函数
	err = rsa.VerifyPKCS1v15(publicKey_, crypto.SHA512, hashText[:], sigText)
	if err != nil {
		return false, err
	}
	return true, nil
}
