/*******************************************************************************
The MIT License (MIT)

Copyright (c) 2014 Hajime Nakagami

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
the Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*******************************************************************************/

package firebirdsql

import (
	"bytes"
	"crypto/sha1"
	"github.com/cznic/mathutil"
	"math/big"
	"math/rand"
	"time"
)

const (
	SRP_KEY_SIZE      = 128
	SRP_SALT_SIZE     = 32
	DEBUG_PRIVATE_KEY = "60975527035CF2AD1989806F0407210BC81EDC04E2762A56AFD529DDDA2D4393"
	DEBUG_SRP         = false
)

func bigFromHexString(s string) *big.Int {
	ret := new(big.Int)
	ret.SetString(s, 16)
	return ret
}

func bigFromString(s string) *big.Int {
	ret := new(big.Int)
	ret.SetString(s, 10)
	return ret
}

func bigToSha1(n *big.Int) []byte {
	sha1 := sha1.New()
	sha1.Write(n.Bytes())

	return sha1.Sum(nil)
}

func pad(v *big.Int) []byte {
	buf := make([]byte, SRP_KEY_SIZE)
	var m big.Int
	var n *big.Int
	n = big.NewInt(0)
	n = n.Add(n, v)

	for i, _ := range buf {
		buf[i] = byte(m.And(m.SetInt64(255), n).Int64())
		n = n.Div(n, m.SetInt64(256))
	}

	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	// skip 0
	var i int
	for i = 0; buf[i] == 0; i++ {
	}
	return buf[i:]
}

func bigToBytes(v *big.Int) []byte {
	buf := pad(v)
	for i, _ := range buf {
		if buf[i] != 0 {
			return buf[i:]
		}
	}

	return buf[:1] // 0
}

func bytesToBig(v []byte) (r *big.Int) {
	m := new(big.Int)
	m.SetInt64(256)
	a := new(big.Int)
	r = new(big.Int)
	r.SetInt64(0)
	for _, b := range v {
		r = r.Mul(r, m)
		r = r.Add(r, a.SetInt64(int64(b)))
	}
	return r
}

func getPrime() (prime *big.Int, g *big.Int, k *big.Int) {
	prime = bigFromHexString("E67D2E994B2F900C3F41F08F5BB2627ED0D49EE1FE767A52EFCD565CD6E768812C3E1E9CE8F0A8BEA6CB13CD29DDEBF7A96D4A93B55D488DF099A15C89DCB0640738EB2CBDD9A8F7BAB561AB1B0DC1C6CDABF303264A08D1BCA932D1F1EE428B619D970F342ABA9A65793B8B2F041AE5364350C16F735F56ECBCA87BD57B29E7")
	g = big.NewInt(2)
	k = bigFromString("1277432915985975349439481660349303019122249719989")
	return
}

func getScramble(keyA *big.Int, keyB *big.Int) *big.Int {
	// keyA:A client public ephemeral values
	// keyB:B server public ephemeral values

	sha1 := sha1.New()
	sha1.Write(pad(keyA))
	sha1.Write(pad(keyB))

	return bytesToBig(sha1.Sum(nil))
}

func getStringHash(s string) *big.Int {
	hash := sha1.New()
	hash.Write(bytes.NewBufferString(s).Bytes())
	return bytesToBig(hash.Sum(nil))
}

func getUserHash(salt []byte, user string, password string) *big.Int {
	hash1 := sha1.New()
	hash1.Write(bytes.NewBufferString(user + ":" + password).Bytes())
	hash2 := sha1.New()
	hash2.Write(salt)
	hash2.Write(hash1.Sum(nil))
	return bytesToBig(hash2.Sum(nil))
}

func getClientSeed() (keyA *big.Int, keya *big.Int) {
	prime, g, _ := getPrime()
	if DEBUG_SRP {
		keya = bigFromString(DEBUG_PRIVATE_KEY)
	} else {
		keya = new(big.Int).Rand(rand.New(rand.NewSource(time.Now().UnixNano())),
			bigFromString("340282366920938463463374607431768211456")) // 1 << 128
	}

	keyA = mathutil.ModPowBigInt(g, keya, prime)
	return
}

func getSalt() []byte {
	buf := make([]byte, SRP_SALT_SIZE)
	if DEBUG_SRP == false {
		for i, _ := range buf {
			buf[i] = byte(rand.Intn(256))
		}
	}
	return buf
}

func getVerifier(user string, password string, salt []byte) *big.Int {
	prime, g, _ := getPrime()
	x := getUserHash(salt, user, password)
	return mathutil.ModPowBigInt(g, x, prime)
}

func getServerSeed(v *big.Int) (keyB *big.Int, keyb *big.Int) {
	prime, g, k := getPrime()
	keyb = new(big.Int).Rand(rand.New(rand.NewSource(time.Now().UnixNano())),
		bigFromString("340282366920938463463374607431768211456")) // 1 << 128
	gb := mathutil.ModPowBigInt(g, keyb, prime)              // gb = pow(g, b, N)
	kv := new(big.Int).Mod(new(big.Int).Mul(k, v), prime)    // kv = (k * v) % N
	keyB = new(big.Int).Mod(new(big.Int).Add(kv, gb), prime) // B = (kv + gb) % N
	return
}

func getClientSession(user string, password string, salt []byte, keyA *big.Int, keyB *big.Int, keya *big.Int) []byte {
	prime, g, k := getPrime()
	u := getScramble(keyA, keyB)
	x := getUserHash(salt, user, password)
	gx := mathutil.ModPowBigInt(g, x, prime)                     // gx = pow(g, x, N)
	kgx := new(big.Int).Mod(new(big.Int).Mul(k, gx), prime)      // kgx = (k * gx) % N
	diff := new(big.Int).Mod(new(big.Int).Sub(keyB, kgx), prime) // diff = (B - kgx) % N
	ux := new(big.Int).Mod(new(big.Int).Mul(u, x), prime)        // ux = (u * x) % N
	aux := new(big.Int).Mod(new(big.Int).Add(keya, ux), prime)   // aux = (a + ux) % N
	sessionSecret := mathutil.ModPowBigInt(diff, aux, prime)     // (B - kg^x) ^ (a + ux)

	return bigToSha1(sessionSecret)
}

func getServerSession(user string, password string, salt []byte, keyA *big.Int, keyB *big.Int, keyb *big.Int) []byte {
	prime, _, _ := getPrime()
	u := getScramble(keyA, keyB)
	v := getVerifier(user, password, salt)
	vu := mathutil.ModPowBigInt(v, u, prime)
	avu := new(big.Int).Mod(new(big.Int).Mul(keyA, vu), prime)
	sessionSecret := mathutil.ModPowBigInt(avu, keyb, prime)
	return bigToSha1(sessionSecret)
}

func getClientProof(user string, password string, salt []byte, keyA *big.Int, keyB *big.Int, keya *big.Int) (keyM []byte, keyK []byte) {
	// M = H(H(N) xor H(g), H(I), s, A, B, K)
	prime, g, _ := getPrime()
	keyK = getClientSession(user, password, salt, keyA, keyB, keya)

	n1 := bytesToBig(bigToSha1(prime))
	n2 := bytesToBig(bigToSha1(g))
	n3 := mathutil.ModPowBigInt(n1, n2, prime)
	n4 := getStringHash(user)
	sha1 := sha1.New()
	sha1.Write(n3.Bytes())
	sha1.Write(n4.Bytes())
	sha1.Write(salt)
	sha1.Write(keyA.Bytes())
	sha1.Write(keyB.Bytes())
	sha1.Write(keyK)
	keyM = sha1.Sum(nil)

	return keyM, keyK
}
