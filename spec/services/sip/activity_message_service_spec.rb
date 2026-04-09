require 'rails_helper'

RSpec.describe Sip::ActivityMessageService, type: :service do
  let(:account) { create(:account) }
  let(:inbox) { create(:channel_sip, account: account).inbox }
  let(:conversation) { create(:conversation, account: account, inbox: inbox) }
  let(:user) { create(:user, account: account) }
  let(:call_id) { 'call-123' }

  describe '#perform' do
    context 'with agent actions' do
      it 'enqueues an activity message for agent answered events' do
        expect do
          described_class.new(
            conversation: conversation, action: :answered, direction: 'inbound', user: user, call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "#{user.name} answered the call (#{call_id})")
        )
      end

      it 'enqueues an activity message for agent rejected events' do
        expect do
          described_class.new(
            conversation: conversation, action: :rejected, direction: 'inbound', user: user, call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "#{user.name} rejected the call (#{call_id})")
        )
      end

      it 'enqueues an activity message for agent canceled events' do
        expect do
          described_class.new(
            conversation: conversation, action: :canceled, direction: 'outbound', user: user, call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "#{user.name} canceled the call (#{call_id})")
        )
      end

      it 'enqueues an activity message for agent ended (hangup) events' do
        expect do
          described_class.new(
            conversation: conversation, action: :ended, direction: 'inbound', user: user, call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "#{user.name} ended the call (#{call_id})")
        )
      end

      it 'does not enqueue when user is nil for agent actions' do
        expect do
          described_class.new(
            conversation: conversation, action: :answered, direction: 'inbound', call_id: call_id
          ).perform
        end.not_to have_enqueued_job(Conversations::ActivityMessageJob)
      end

      it 'does not enqueue for an unrecognized agent action' do
        expect do
          described_class.new(
            conversation: conversation, action: :unknown_action, direction: 'inbound', user: user, call_id: call_id
          ).perform
        end.not_to have_enqueued_job(Conversations::ActivityMessageJob)
      end
    end

    context 'with terminal reasons' do
      it 'enqueues a direction-aware message for remote-canceled (outbound)' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'outbound', reason: 'remote-canceled', call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "Recipient canceled the call (#{call_id})")
        )
      end

      it 'enqueues a direction-aware message for remote-canceled (inbound)' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'inbound', reason: 'remote-canceled', call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "Caller canceled the call (#{call_id})")
        )
      end

      it 'enqueues a message for remote-hangup (outbound)' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'outbound', reason: 'remote-hangup', call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "Recipient ended the call (#{call_id})")
        )
      end

      it 'enqueues a message for remote-hangup (inbound)' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'inbound', reason: 'remote-hangup', call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "Caller ended the call (#{call_id})")
        )
      end

      it 'enqueues a busy message' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'outbound', reason: 'busy', call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "Recipient was busy (#{call_id})")
        )
      end

      it 'enqueues a no-answer message' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'inbound', reason: 'no-answer', call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "The call was not answered (#{call_id})")
        )
      end

      it 'enqueues a not-found message' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'outbound', reason: 'not-found', call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "Recipient could not be reached (#{call_id})")
        )
      end

      it 'enqueues a service-unavailable message' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'outbound', reason: 'service-unavailable', call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "Call failed because the service was unavailable (#{call_id})")
        )
      end

      it 'enqueues a generic failed message for sip- prefixed reasons' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'outbound', reason: 'sip-503', call_id: call_id
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "Call failed (#{call_id})")
        )
      end

      it 'does not enqueue when terminal action has no valid reason' do
        expect do
          described_class.new(
            conversation: conversation, action: :terminal, direction: 'outbound', reason: nil, call_id: call_id
          ).perform
        end.not_to have_enqueued_job(Conversations::ActivityMessageJob)
      end
    end

    context 'without call_id' do
      it 'omits the call_id suffix from the message' do
        expect do
          described_class.new(
            conversation: conversation, action: :answered, direction: 'inbound', user: user
          ).perform
        end.to have_enqueued_job(Conversations::ActivityMessageJob).with(
          conversation,
          hash_including(content: "#{user.name} answered the call")
        )
      end
    end
  end
end
