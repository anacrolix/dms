package webadmin

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"

	"github.com/anacrolix/dms/dlna/dms"
	"github.com/gin-gonic/gin"
)

func randomPassword() string {
	length := 32
	randomArray := make([]byte, length)
	rand.Read(randomArray)

	data := base64.StdEncoding.EncodeToString(randomArray)
	return data[1:8]
}

func removeIpIfPresent(iPNet []*net.IPNet, s_ip string) ([]*net.IPNet, int) {
	hit := 0
	ip := net.ParseIP(s_ip)

	if ip == nil {
		return iPNet, -1
	}

	for i, ipnet := range iPNet {
		if ipnet.IP.Equal(ip) {
			hit = hit + 1
			iPNet = append(iPNet[:i], iPNet[i+1:]...)
		}
	}

	return iPNet, hit
}

func deleterHelper(c *gin.Context, iPNet []*net.IPNet, s_ip string) ([]*net.IPNet, int) {
	iPNet, nhit := removeIpIfPresent(iPNet, s_ip)

	if nhit == 0 {
		c.JSON(404, gin.H{
			"message": "Not found",
		})
	} else if nhit < 0 {
		c.JSON(400, gin.H{
			"message": "The ip is not valid",
		})
	} else {
		c.JSON(200, gin.H{
			"message": "The element was removed",
			"count":   nhit,
		})

	}
	return iPNet, nhit
}

func WebadminStartAsync(sharedSettings *dms.Server) {

	if len(sharedSettings.AdminPassword) == 0 {
		sharedSettings.AdminPassword = randomPassword()
		fmt.Printf("Password was set to %s ", sharedSettings.AdminPassword)
	}

	router := gin.Default()

	router.Static("/webassets", "./webassets")

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	router.GET("/blacklist", func(c *gin.Context) {
		c.JSON(200, sharedSettings.BlacklistedIpNets)
	})

	router.GET("/whitelist", func(c *gin.Context) {
		c.JSON(200, sharedSettings.AllowedIpNets)
	})

	router.GET("/refused", func(c *gin.Context) {
		c.JSON(200, sharedSettings.RefusedClients)
	})

	router.GET("/status", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"allowed":   sharedSettings.AllowedClients,
			"blacklist": sharedSettings.BlacklistedIpNets,
			"refused":   sharedSettings.RefusedClients,
			"whitelist": sharedSettings.AllowedIpNets,
		})
	})

	router.DELETE("/blacklist/:id", func(c *gin.Context) {
		sharedSettings.BlacklistedIpNets, _ = deleterHelper(c, sharedSettings.BlacklistedIpNets, c.Param("id"))
	})

	router.DELETE("/whitelist/:id", func(c *gin.Context) {
		sharedSettings.BlacklistedIpNets, _ = deleterHelper(c, sharedSettings.AllowedIpNets, c.Param("id"))
	})

	router.PUT("/blacklist/:id", func(c *gin.Context) {
		xx, x := removeIpIfPresent(sharedSettings.AllowedIpNets, c.Param("id"))
		sharedSettings.AllowedIpNets = xx

		if x < 0 {
			c.JSON(400, gin.H{
				"message": "The ip is not valid",
			})
			return
		}

		var item net.IPNet
		item.IP = net.ParseIP(c.Param("id"))
		item.Mask = net.IPv4Mask(255, 255, 255, 255)
		sharedSettings.BlacklistedIpNets = append(sharedSettings.BlacklistedIpNets, &item)

		c.JSON(200, gin.H{
			"message": "Ok",
		})
	})

	router.PUT("/whitelist/:id", func(c *gin.Context) {
		xx, x := removeIpIfPresent(sharedSettings.BlacklistedIpNets, c.Param("id"))
		sharedSettings.BlacklistedIpNets = xx

		if x < 0 {
			c.JSON(400, gin.H{
				"message": "The ip is not valid",
			})
			return
		}

		var item net.IPNet
		item.IP = net.ParseIP(c.Param("id"))
		item.Mask = net.IPv4Mask(255, 255, 255, 255)
		sharedSettings.AllowedIpNets = append(sharedSettings.AllowedIpNets, &item)

		c.JSON(200, gin.H{
			"message": "Ok",
		})
	})

	router.Run() // listen and serve on 0.0.0.0:8080
}
