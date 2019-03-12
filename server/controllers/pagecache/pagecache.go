package pagecache

import (
	"github.com/getupandgo/gocache/utils/cache"
	"github.com/getupandgo/gocache/utils/structs"
	"github.com/gin-gonic/gin"
	"net/http"
)

type CacheController struct {
	cacheClient cache.CacheClient
}

func Init(cc cache.CacheClient) *CacheController {
	return &CacheController{cc}
}

func (ctrl *CacheController) GetPage(c *gin.Context) {
	pg := c.Query("url")

	cont, err := ctrl.cacheClient.GetPage(pg)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, cont)
}

func (ctrl *CacheController) UpsertPage(c *gin.Context) {
	newPage := &structs.Page{}

	if err := c.BindJSON(newPage); err != nil {
		c.Error(err)
		return
	}

	if err := ctrl.cacheClient.UpsertPage(newPage); err != nil {
		c.Error(err)
		return
	}

	c.String(http.StatusOK, newPage.Url)
}

func (ctrl *CacheController) DeletePage(c *gin.Context) {
	removePage := &structs.RemovePageBody{}

	if err := c.BindJSON(removePage); err != nil {
		c.Error(err)
		return
	}

	ctrl.cacheClient.RemovePage(removePage.Url)
}

func (ctrl *CacheController) GetTopPages(c *gin.Context) {
	top, err := ctrl.cacheClient.GetTopPages()
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, top)
}
