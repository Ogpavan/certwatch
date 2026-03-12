import { Modal, Form, Input, Select, InputNumber } from "antd";

const { TextArea } = Input;

export default function AddDomainModal({ open, onCancel, onCreate, projects = [] }) {
  const [form] = Form.useForm();

  return (
    <Modal
      title="Add Domain"
      open={open}
      onCancel={onCancel}
      onOk={() => {
        form
          .validateFields()
          .then((values) => {
            onCreate(values);
            form.resetFields();
          })
          .catch(() => {});
      }}
      okText="Add"
    >
      <Form layout="vertical" form={form}>
        <Form.Item name="project" label="Project" rules={[{ required: true, message: "Select a project" }]}
        >
          <Select
            placeholder="Select project"
            options={projects.map((project) => ({ value: project.id, label: project.name }))}
          />
        </Form.Item>
        <Form.Item
          name="domain"
          label="Domain name"
          dependencies={["bulk"]}
          rules={[
            {
              validator: (_, value) => {
                const bulkValue = form.getFieldValue("bulk");
                if ((!value || !value.trim()) && (!bulkValue || !bulkValue.trim())) {
                  return Promise.reject(new Error("Enter a domain or paste bulk entries"));
                }
                return Promise.resolve();
              }
            }
          ]}
        >
          <Input placeholder="e.g. api.acme.com" />
        </Form.Item>
        <Form.Item name="port" label="Port" initialValue={443}>
          <InputNumber min={1} max={65535} style={{ width: "100%" }} />
        </Form.Item>
        <Form.Item name="bulk" label="Bulk entry">
          <TextArea rows={4} placeholder="Paste multiple domains, one per line" />
        </Form.Item>
      </Form>
    </Modal>
  );
}
