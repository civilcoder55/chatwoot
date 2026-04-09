class CreateSipCalls < ActiveRecord::Migration[7.0]
  def change
    create_table :sip_calls do |t|
      t.bigint :account_id, null: false
      t.bigint :inbox_id, null: false
      t.bigint :conversation_id, null: false
      t.bigint :accepted_by_agent_id
      t.bigint :message_id
      t.string :call_id, null: false
      t.string :direction, null: false
      t.string :status, null: false, default: 'ringing'
      t.integer :duration_seconds
      t.string :end_reason
      t.jsonb :meta, null: false, default: {}

      t.timestamps
    end

    add_index :sip_calls, :call_id, unique: true
    add_index :sip_calls, %i[account_id conversation_id]
    add_index :sip_calls, %i[inbox_id status]
    add_index :sip_calls, :message_id
  end
end
