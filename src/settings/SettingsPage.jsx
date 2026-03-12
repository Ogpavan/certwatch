import { Card, Input, InputNumber, Switch, Space, Tag, Button, message, TimePicker } from "antd";
import { useEffect, useState } from "react";
import dayjs from "dayjs";
import { api } from "../services/api.js";

export default function SettingsPage() {
  const DEFAULT_SCHEDULE = "03:00";
  const [notifyDays, setNotifyDays] = useState(["30", "14", "7", "3"]);
  const [nextDay, setNextDay] = useState(10);
  const [emailEnabled, setEmailEnabled] = useState(true);
  const [emailRecipients, setEmailRecipients] = useState([]);
  const [emailInput, setEmailInput] = useState("");
  const [scheduleTime, setScheduleTime] = useState(DEFAULT_SCHEDULE);
  const [lastScheduleRun, setLastScheduleRun] = useState(null);

  const handleAddDay = () => {
    if (!nextDay || nextDay < 1) {
      message.warning("Enter a positive day count");
      return;
    }
    const value = nextDay.toString();
    if (notifyDays.includes(value)) {
      message.info("Threshold already added");
      return;
    }
    const updated = [...notifyDays, value].sort((a, b) => Number(a) - Number(b));
    setNotifyDays(updated);
    message.success(`Notification set for ${value} day${value === "1" ? "" : "s"} left`);
    persistSettings(emailEnabled, emailRecipients, updated, scheduleTime);
  };

  const removeDay = (value) => {
    const updated = notifyDays.filter((item) => item !== value);
    setNotifyDays(updated);
    persistSettings(emailEnabled, emailRecipients, updated, scheduleTime);
  };

  useEffect(() => {
    let active = true;
    api
      .getNotificationSettings()
      .then((data) => {
        if (!active) return;
        setEmailEnabled(Boolean(data?.email_enabled));
        setEmailRecipients(Array.isArray(data?.email_recipients) ? data.email_recipients : []);
        if (Array.isArray(data?.notify_days)) {
          const normalized = data.notify_days.map((value) => value.toString());
          setNotifyDays(normalized);
        }
        const scheduled = typeof data?.schedule_time === "string" && data.schedule_time ? data.schedule_time : DEFAULT_SCHEDULE;
        setScheduleTime(scheduled);
        setLastScheduleRun(data?.last_scanned_at ?? null);
      })
      .catch((err) => {
        if (!active) return;
        message.error(err?.message || "Failed to load notification settings");
      });
    return () => {
      active = false;
    };
  }, []);

  const persistSettings = async (enabled, recipients, days, schedule) => {
    try {
      await api.updateNotificationSettings({
        email_enabled: enabled,
        email_recipients: recipients,
        notify_days: days,
        schedule_time: schedule || DEFAULT_SCHEDULE
      });
    } catch (err) {
      message.error(err?.message || "Failed to save notification settings");
    }
  };

  const handleAddEmail = () => {
    const value = emailInput.trim().toLowerCase();
    if (!value) {
      message.warning("Enter an email address");
      return;
    }
    if (!/^\S+@\S+\.\S+$/.test(value)) {
      message.warning("Enter a valid email address");
      return;
    }
    if (emailRecipients.includes(value)) {
      message.info("Email already added");
      return;
    }
    const updated = [...emailRecipients, value];
    setEmailRecipients(updated);
    setEmailInput("");
    message.success("Recipient added");
    persistSettings(emailEnabled, updated, notifyDays, scheduleTime);
  };

  const removeEmail = (value) => {
    const updated = emailRecipients.filter((item) => item !== value);
    setEmailRecipients(updated);
    persistSettings(emailEnabled, updated, notifyDays, scheduleTime);
  };

  const handleScheduleChange = (value) => {
    const normalized = value || DEFAULT_SCHEDULE;
    setScheduleTime(normalized);
    persistSettings(emailEnabled, emailRecipients, notifyDays, normalized);
  };

  const scheduleToDayjs = (value) => {
    const fallback = DEFAULT_SCHEDULE.split(":");
    const parts = (value || DEFAULT_SCHEDULE).split(":");
    const hour = Number(parts[0]);
    const minute = Number(parts[1]);
    const safeHour = Number.isInteger(hour) && hour >= 0 && hour <= 23 ? hour : Number(fallback[0]);
    const safeMinute = Number.isInteger(minute) && minute >= 0 && minute <= 59 ? minute : Number(fallback[1]);
    return dayjs().hour(safeHour).minute(safeMinute).second(0);
  };

  const formatDateTime12 = (value) => {
    if (!value) return "-";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "-";
    const datePart = date
      .toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" })
      .replace(/ /g, "-");
    const timePart = date.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit", hour12: true });
    return `${datePart} ${timePart}`;
  };

  return (
    <div>
      <div className="page-header">
        <div className="section-title">Settings</div>
      </div>

      <Card className="card-surface" style={{ marginBottom: 16 }}>
        <div className="section-title">SSL expiry notifications</div>
        <p>Enter the number of days remaining before you want to receive an alert.</p>
        <Space wrap>
          {notifyDays.map((day) => (
            <Tag
              key={day}
              closable
              onClose={() => removeDay(day)}
              color="#3B82F6"
            >
              Alert at {day} day{day === "1" ? "" : "s"} left
            </Tag>
          ))}
        </Space>
        <Space style={{ marginTop: 12 }}>
          <InputNumber
            min={1}
            max={365}
            value={nextDay}
            onChange={(value) => setNextDay(value ?? 1)}
          />
          <Button type="primary" size="small" onClick={handleAddDay}>
            Add threshold
          </Button>
        </Space>
      </Card>

      <Card className="card-surface" style={{ marginBottom: 16 }}>
        <div className="section-title">Scan schedule</div>
        <p>Select the time of day when automatic scans should run.</p>
        <Space direction="vertical">
          <TimePicker
            use12Hours
            format="hh:mm A"
            value={scheduleToDayjs(scheduleTime)}
            onChange={(value) => handleScheduleChange(value ? value.format("HH:mm") : DEFAULT_SCHEDULE)}
            style={{ width: 160 }}
          />
          <span style={{ color: "#475569" }}>
            Last scheduled run: {lastScheduleRun ? formatDateTime12(lastScheduleRun) : "Not run yet"}
          </span>
        </Space>
      </Card>

      <Card className="card-surface">
        <div className="section-title">Notification channels</div>
        <Space direction="vertical" size={12}>
          <Space>
            <Switch
              checked={emailEnabled}
              onChange={(checked) => {
                setEmailEnabled(checked);
                persistSettings(checked, emailRecipients, notifyDays, scheduleTime);
              }}
            />
            <span>Email</span>
          </Space>
          <div>
            <div style={{ marginBottom: 8 }}>Email recipients</div>
            <Space wrap>
              {emailRecipients.length === 0 && (
                <span style={{ color: "#64748B" }}>No recipients added.</span>
              )}
              {emailRecipients.map((email) => (
                <Tag
                  key={email}
                  closable={emailEnabled}
                  onClose={() => removeEmail(email)}
                  color="#10B981"
                >
                  {email}
                </Tag>
              ))}
            </Space>
            <Space style={{ marginTop: 12 }}>
              <Input
                placeholder="name@company.com"
                value={emailInput}
                onChange={(event) => setEmailInput(event.target.value)}
                disabled={!emailEnabled}
                style={{ width: 260 }}
                onPressEnter={handleAddEmail}
              />
              <Button type="primary" size="small" onClick={handleAddEmail} disabled={!emailEnabled}>
                Add email
              </Button>
            </Space>
          </div>
          <Space>
            <Switch />
            <span>Webhook</span>
          </Space>
          <Space>
            <Switch />
            <span>Telegram</span>
          </Space>
        </Space>
      </Card>
    </div>
  );
}
