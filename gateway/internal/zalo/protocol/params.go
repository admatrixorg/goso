package protocol

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The Zalo web client signs each API call with a per-request key derived from a
// freshly built zcid. Without that layer Zalo answers with a session key error,
// even when the stored cookies are valid.
const (
	// DefaultComputerName is the machine name the web client reports.
	DefaultComputerName = "Web"
	// DefaultEncryptVer is the parameter encryption version.
	DefaultEncryptVer = "v2"
	// DefaultZCIDKey is the fixed AES key Zalo uses to wrap the zcid.
	DefaultZCIDKey = "3FC4F0D2AB50057BCE0D90D9187A22B1"
)

// encryptResult carries the request key and the encrypted payload from one
// parameter build.
type encryptResult struct {
	key     string
	encData string
}

// EncryptParam builds the signed parameter set an API call carries, and returns
// the request key needed to open that call's reply.
func (s *Session) EncryptParam(typeStr string) (map[string]string, string, error) {
	if s == nil {
		return nil, "", fmt.Errorf("zalo personal: session is not ready")
	}
	data := map[string]any{
		"computer_name": DefaultComputerName,
		"imei":          s.IMEI,
		"language":      s.Language,
		"ts":            time.Now().UnixMilli(),
	}
	zcid, zcidExt, key, err := encryptParams(s.IMEI, data)
	if err != nil {
		return nil, "", err
	}
	params := map[string]string{
		"zcid":           zcid,
		"enc_ver":        DefaultEncryptVer,
		"zcid_ext":       zcidExt,
		"params":         key.encData,
		"type":           strconv.Itoa(DefaultAPIType),
		"client_version": strconv.Itoa(DefaultAPIVersion),
	}
	if typeStr == "getserverinfo" {
		params["signkey"] = generateSignKey(typeStr, map[string]any{
			"imei":           s.IMEI,
			"type":           DefaultAPIType,
			"client_version": DefaultAPIVersion,
			"computer_name":  DefaultComputerName,
		})
	} else {
		anyParams := make(map[string]any, len(params))
		for k, v := range params {
			anyParams[k] = v
		}
		params["signkey"] = generateSignKey(typeStr, anyParams)
	}
	return params, key.key, nil
}

// generateSignKey is an md5 over the type marker and the sorted parameter values.
func generateSignKey(typeStr string, params map[string]any) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var signStr strings.Builder
	signStr.WriteString("zsecure" + typeStr)
	for _, k := range keys {
		if v := params[k]; v != nil {
			signStr.WriteString(convertToString(v))
		}
	}
	sum := md5.Sum([]byte(signStr.String()))
	return hex.EncodeToString(sum[:])
}

// encryptParams builds the zcid, derives the request key and crypts the payload.
func encryptParams(imei string, data map[string]any) (zcid, zcidExt string, result *encryptResult, err error) {
	ts := time.Now().UnixMilli()
	zcidExt = randomHexString(6, 12)

	zcidData := fmt.Sprintf("%d,%s,%d", DefaultAPIType, imei, ts)
	zcidRaw, cerr := EncodeAESCBC([]byte(DefaultZCIDKey), zcidData, true)
	if cerr != nil {
		return "", "", nil, fmt.Errorf("zalo personal: build zcid: %w", cerr)
	}
	zcid = strings.ToUpper(zcidRaw)

	key, kerr := deriveEncryptKey(zcidExt, zcid)
	if kerr != nil {
		return "", "", nil, kerr
	}
	blob, merr := json.Marshal(data)
	if merr != nil {
		return "", "", nil, merr
	}
	encData, eerr := EncodeAESCBC([]byte(key), string(blob), false)
	if eerr != nil {
		return "", "", nil, fmt.Errorf("zalo personal: encrypt params: %w", eerr)
	}
	return zcid, zcidExt, &encryptResult{key: key, encData: encData}, nil
}

// DeriveEncryptKey mixes the random extension with the zcid into the request key.
func DeriveEncryptKey(ext, id string) (string, error) {
	return deriveEncryptKey(ext, id)
}

// deriveEncryptKey mixes the random extension with the zcid into the request key.
func deriveEncryptKey(ext, id string) (string, error) {
	sum := md5.Sum([]byte(ext))
	nUpper := strings.ToUpper(hex.EncodeToString(sum[:]))
	evenE, _ := processStr(nUpper)
	evenI, oddI := processStr(id)
	if len(evenE) == 0 || len(evenI) == 0 || len(oddI) == 0 {
		return "", fmt.Errorf("zalo personal: key derivation needs a zcid and its extension")
	}
	var b strings.Builder
	b.WriteString(joinFirst(evenE, 8))
	b.WriteString(joinFirst(evenI, 12))
	b.WriteString(joinFirst(reverseCopy(oddI), 12))
	return b.String(), nil
}

// processStr splits a string into its even and odd index characters.
func processStr(s string) (even, odd []string) {
	for i, r := range s {
		if i%2 == 0 {
			even = append(even, string(r))
		} else {
			odd = append(odd, string(r))
		}
	}
	return
}

func joinFirst(parts []string, n int) string {
	if n > len(parts) {
		n = len(parts)
	}
	return strings.Join(parts[:n], "")
}

func reverseCopy(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func randomHexString(minLen, maxLen int) string {
	length := minLen
	if maxLen > minLen {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(maxLen-minLen+1)))
		length = minLen + int(n.Int64())
	}
	byteLen := (length + 1) / 2
	buf := make([]byte, byteLen)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)[:length]
}

func convertToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprint(val)
	}
}
