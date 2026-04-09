class CreateChannelSip < ActiveRecord::Migration[7.0]
  def change
    create_table :channel_sip do |t|
      t.string :phone_number, null: false
      t.jsonb :provider_config, null: false, default: {}
      t.integer :account_id, null: false

      t.timestamps
    end

    add_index :channel_sip, :phone_number, unique: true
    add_index :channel_sip, :account_id
  end
end
