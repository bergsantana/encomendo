// Package notify is the adapter that sends messages to people: invitations
// and password reset codes by email in milestone 2 (ADR 0011), and WhatsApp
// links later. Text is pt-BR. It builds messages and hands them over; who
// may be written to, and how often, is decided by the use cases and the
// rate limits.
package notify
