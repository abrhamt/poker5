package handlers

import (
	"bufio"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
	"github.com/zuse/poker5/go-poker/services"
)

type SSEHandler struct {
	hub *services.SSEHub
}

func NewSSEHandler(hub *services.SSEHub) *SSEHandler {
	return &SSEHandler{hub: hub}
}

func (h *SSEHandler) HandleEvents(c fiber.Ctx) error {
	tableID := c.Query("table_id")
	if tableID == "" {
		tableID = "demo"
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	ctx := c.Context()

	c.RequestCtx().SetBodyStreamWriter(fasthttp.StreamWriter(func(bw *bufio.Writer) {
		ch, unsubscribe := h.hub.Subscribe(tableID)
		defer unsubscribe()

		_, _ = bw.WriteString("event: connected\ndata: {\"status\":\"ok\"}\n\n")
		_ = bw.Flush()

		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-ch:
				payload := services.FormatSSEPayload(msg)
				if _, err := bw.WriteString(payload); err != nil {
					return
				}
				if err := bw.Flush(); err != nil {
					return
				}
			}
		}
	}))

	return nil
}
